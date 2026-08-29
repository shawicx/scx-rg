package tui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"scx-rg/internal/logs"
)

// 实时多面板视图（scx-rg docker / k8s 默认）：每容器一个 logs -f 流进程，
// 行经 100ms 批量窗口聚合为 liveLinesMsg，避免高频日志逐行打爆 Update。
// 面板缓冲只保留最新 logWindow 行（与日志搜索窗口一致）。

const (
	liveBatchInterval = 100 * time.Millisecond // 批量聚合窗口
	liveBatchFlush    = 200                    // 攒够即冲刷，不等窗口
	liveMaxPanels     = 4                      // 分屏上限（2×2）
	// liveDrainWait onceMode（--once / 测试 drain）下单次读实时管线的限时：
	// 实时读链是自续链（每条消息后续读一条），流不收束就永不停止，同步
	// 驱动器会卡死在阻塞读上——限时后返回 nil 收束读链。真实事件循环
	// （onceMode=false）仍是纯阻塞读，行为不受影响。
	liveDrainWait = 500 * time.Millisecond
)

// livePanel 单容器实时面板：缓冲与滚动状态只允许 Update goroutine 读写
// （并发约束见计划 Global Constraints）。
type livePanel struct {
	target logs.Target
	path   string // tee 落盘路径（y 复制搜索命令用）
	buf    []string
	vp     viewport.Model
	follow bool // 贴底跟随：上翻暂停，回底恢复
	exited bool // 流进程已收束（容器停止/出错）
	w, h   int  // 面板外框尺寸（liveView 渲染用）
}

// appendLines 追加日志行并维持环形窗口。
func (p *livePanel) appendLines(lines []string) {
	p.buf = append(p.buf, lines...)
	if n := len(p.buf); n > logWindow {
		p.buf = p.buf[n-logWindow:]
	}
}

// rebuild 重建面板内容：跟随态贴底，暂停态保持当前偏移。
func (p *livePanel) rebuild() {
	y := p.vp.YOffset()
	p.vp.SetContentLines(p.buf)
	if p.follow || len(p.buf) == 0 {
		p.vp.SetYOffset(max(0, len(p.buf)-p.vp.Height()))
	} else {
		p.vp.SetYOffset(y)
	}
}

// liveRows n 个面板的分行布局（每元素=该行并排面板数）。
// 1→全屏 2→上下 3→上1下2 4→田字；越界返回 nil。
func liveRows(n int) []int {
	switch n {
	case 1:
		return []int{1}
	case 2:
		return []int{1, 1}
	case 3:
		return []int{1, 2}
	case 4:
		return []int{2, 2}
	default:
		return nil
	}
}

type (
	// liveLinesMsg 面板批量新行（seq 防跨会话串扰）。
	liveLinesMsg struct {
		seq   int
		panel int
		lines []string
	}
	// liveDoneMsg 面板流收束（err=nil 容器停止；err!=nil 启动/运行失败）。
	liveDoneMsg struct {
		seq   int
		panel int
		err   error
	}
)

// sendLive 向管线发消息：会话已无人读（退出/重选）时借 ctx.Done 放弃，
// 防止 goroutine 永久阻塞泄漏。
func sendLive(ctx context.Context, ch chan<- tea.Msg, msg tea.Msg) {
	select {
	case ch <- msg:
	case <-ctx.Done():
	}
}

// waitLiveLines 阻塞等一条实时管线消息（goroutine→Update 的桥）。
// onceMode 下限时等待（见 liveDrainWait），防止同步驱动器在长驻流上卡死。
func (m *Model) waitLiveLines(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if m.onceMode {
			select {
			case msg := <-ch:
				return msg
			case <-time.After(liveDrainWait):
				return nil
			}
		}
		return <-ch
	}
}

// allLiveDone 全部面板的流都已收束。
func (m *Model) allLiveDone() bool {
	for _, p := range m.livePanels {
		if !p.exited {
			return false
		}
	}
	return true
}

// startLive 为每个目标启动流进程与批量管线，切入实时模式。
// 每面板两个 goroutine：stream（回调推行）与 batcher（唯一 liveCh 发送者，
// 保证「新行消息先于退出消息」的顺序——退出消息若抢在残余行前入队，
// allLiveDone 会提前收束读链，尾部日志行滞留管道丢失）。
func (m *Model) startLive(targets []logs.Target) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.liveCancel = cancel
	m.liveMode = true
	m.liveFocus = 0
	m.liveSeq++
	seq := m.liveSeq
	m.liveCh = make(chan tea.Msg, 64)
	// goroutine 只捕获局部变量：读 m.liveCh 字段会与重进实时时的
	// 重新赋值构成数据竞态（重入 startLive 在 Update goroutine 写）
	ch := m.liveCh
	tail := m.cfg.LogTail
	if tail <= 0 {
		tail = defaultLogTail
	}
	stream := m.cfg.StreamLog
	if stream == nil {
		stream = logs.Stream
	}
	for i, t := range targets {
		p := &livePanel{target: t, path: logs.LivePath(m.cfg.LiveDir, t), follow: true}
		if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
			p.buf = []string{"⚠ " + err.Error()}
			p.exited = true
			m.livePanels = append(m.livePanels, p)
			continue
		}
		m.livePanels = append(m.livePanels, p)
		linesCh := make(chan string, 256)
		exitCh := make(chan error, 1)
		go func() { exitCh <- stream(ctx, t, tail, p.path, func(l string) { linesCh <- l }) }()
		go func(i int) {
			// exitErr 须先于捕获它的 defer 声明（词法作用域），return
			// 路径先赋值再执行 defer，顺序正确
			var exitErr error
			defer func() { sendLive(ctx, ch, liveDoneMsg{seq: seq, panel: i, err: exitErr}) }()
			pending := make([]string, 0, liveBatchFlush)
			tick := time.NewTicker(liveBatchInterval)
			defer tick.Stop()
			flush := func() {
				if len(pending) == 0 {
					return
				}
				sendLive(ctx, ch, liveLinesMsg{seq: seq, panel: i, lines: pending})
				pending = make([]string, 0, liveBatchFlush)
			}
			for {
				select {
				case l := <-linesCh:
					pending = append(pending, l)
					if len(pending) >= liveBatchFlush {
						flush()
					}
				case <-tick.C:
					flush()
				case err := <-exitCh:
					exitErr = err
					// stream 已返回，残余行都在 linesCh 缓冲里：先抽干再收束
					for {
						select {
						case l := <-linesCh:
							pending = append(pending, l)
						default:
							flush()
							return
						}
					}
				}
			}
		}(i)
	}
	m.resizeLivePanels()
	return m.waitLiveLines(ch)
}

// stopLive 停止全部流进程并清实时状态（退出/回选择器）。
func (m *Model) stopLive() {
	if m.liveCancel != nil {
		m.liveCancel()
		m.liveCancel = nil
	}
	m.liveMode = false
	m.livePanels = nil
	m.liveFocus = 0
}

// livePanelColor 面板循环取色（标题/状态点），动态读主题槽位——
// ApplyTheme 换色后无需重建。
// 返回类型是 image/color 的 Color 接口（包级色槽的声明类型）：
// lipgloss v2 的 lipgloss.Color 是构造函数而非类型，无法作签名。
func livePanelColor(i int) color.Color {
	switch i % 4 {
	case 0:
		return colorCyan
	case 1:
		return colorAccent
	case 2:
		return colorOK
	default:
		return colorErr
	}
}

// resizeLivePanels 按终端与面板数计算每面板 viewport（标题 1 行 + 边框 2 行，
// 高度再留 1 行给 Height 补齐的余地）。
// 仅 Update goroutine 执行（进实时 / 窗口变化时调用）。注意 lipgloss v2 的
// Width/Height 是「含边框的整体块尺寸」，故面板外框直接取 colW×rowH，
// 视口内容区再向内扣 2 列边框；末行/末列吸收整除余数，拼行时零缝隙。
func (m *Model) resizeLivePanels() {
	rows := liveRows(len(m.livePanels))
	if len(rows) == 0 {
		return
	}
	H := max(0, m.height-4) // 头部 3 行 + 状态栏 1 行，与 panelH 对齐
	W := m.frameW()
	idx := 0
	for r, cols := range rows {
		rowH := H / len(rows)
		if r == len(rows)-1 {
			rowH = H - rowH*(len(rows)-1) // 末行吸收余数
		}
		for c := 0; c < cols; c++ {
			colW := W / cols
			if c == cols-1 {
				colW = W - colW*(cols-1)
			}
			p := m.livePanels[idx]
			p.w, p.h = colW, rowH
			p.vp = viewport.New(viewport.WithWidth(max(0, colW-2)), viewport.WithHeight(max(0, rowH-4)))
			p.rebuild()
			idx++
		}
	}
}

// liveView 分屏渲染：行内 JoinHorizontal、行间 JoinVertical。
func (m *Model) liveView() string {
	if len(m.livePanels) == 0 {
		return ""
	}
	rows := liveRows(len(m.livePanels))
	var rowStrs []string
	idx := 0
	for _, cols := range rows {
		cells := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			cells = append(cells, m.livePanelView(idx))
			idx++
		}
		rowStrs = append(rowStrs, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rowStrs...)
}

// livePanelView 单面板：标题（状态点 ●/■ + 容器名 + 焦点指示）+ 日志区。
// 焦点面板用激活边框，其余空闲边框。
// 状态点 ●（流存活）/ ■（已收束）属于 East Asian Ambiguous 宽度字符，
// 歧义宽按 2 格的终端里标题行会比 lipgloss 计宽多占 1-2 格（与列表
// 标记 ✓/⚠ 同类既有取舍，见 frame_width_test 注释）。
func (m *Model) livePanelView(i int) string {
	p := m.livePanels[i]
	dot := "●"
	// 先取带面板色副本再 Render（lipgloss.Style 是值类型，复制安全）；
	// stylePanelTitle 包级样式不受 Foreground 副本影响。
	titleStyle := stylePanelTitle.Foreground(livePanelColor(i))
	if p.exited {
		dot = "■"
	}
	title := titleStyle.Render(dot + " " + p.target.Name)
	if i == m.liveFocus {
		title += styleDim.Render(" ◀")
	}
	border := styleBorderIdle
	if i == m.liveFocus {
		border = styleBorderActive
	}
	body := p.vp.View()
	if len(p.buf) == 0 && !p.exited {
		body = stylePlaceholder.Render("等待日志...")
	}
	return border.Width(p.w).Height(p.h).Render(title + "\n" + body)
}

// liveStatus 实时模式状态栏：焦点容器、退出计数、落盘目录与键位提示。
func (m *Model) liveStatus() string {
	left := styleBadgeContent.Render("实时 " + pickerKindLabel(m.pickerKind))
	if m.liveFocus < len(m.livePanels) {
		left += " " + m.livePanels[m.liveFocus].target.Name
	}
	exited := 0
	for _, p := range m.livePanels {
		if p.exited {
			exited++
		}
	}
	if exited > 0 {
		left += styleDim.Render(fmt.Sprintf(" %d 已退出", exited))
	}
	left += styleDim.Render(" · 落盘 " + m.cfg.LiveDir)
	if m.notice != "" {
		left += " / " + styleMatch.Render(m.notice)
	}
	right := "j/k 滚动 / Tab·1-4 焦点 / G 回底 / y 复制搜索命令 / Ctrl+R 重选 / Esc 退出"
	return m.statusLine(left, right)
}

// handleLiveKey 实时视图键位（Task 2 最小集：退出与重选；完整滚动/焦点在后续任务）。
func (m *Model) handleLiveKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		return m, tea.Quit
	case "esc":
		m.shutdown()
		return m, tea.Quit
	case "ctrl+r", "alt+r":
		return m, m.reenterPicker()
	}
	return m, nil
}
