package tui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
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
	p.rebuildAt(p.vp.YOffset())
}

// rebuildAt 以 keep 为暂停态偏移重建。resize 换新视口时旧偏移随旧视口
// 丢失，须在 viewport.New 前取出再传入（直接用 rebuild 会拿到新视口的 0，
// 暂停跟随的上翻位置静默回顶）。SetContentLines 必须先于 SetYOffset：
// 全新视口没有内容时 maxYOffset 恒 0，先设偏移会被 clamp 归零。
func (p *livePanel) rebuildAt(keep int) {
	p.vp.SetContentLines(p.buf)
	if p.follow || len(p.buf) == 0 {
		p.vp.SetYOffset(max(0, len(p.buf)-p.vp.Height()))
	} else {
		p.vp.SetYOffset(keep)
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
	// 防御性停流：picker Enter 与 LiveTargets 直达是不经过 reenterPicker
	// 的多入口调用方，若上一会话的 liveCancel 尚未清掉，直接覆盖会孤立
	// 旧 ctx（无人再取消，goroutine 泄漏）。stopLive 幂等，先停再启。
	m.stopLive()
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
			// 新视口 YOffset 从 0 开始：换建前保存旧偏移，暂停跟随态
			// resize 后不回顶（跟随态 rebuildAt 内部自会贴底，不受影响）
			y := p.vp.YOffset()
			p.w, p.h = colW, rowH
			p.vp = viewport.New(viewport.WithWidth(max(0, colW-2)), viewport.WithHeight(max(0, rowH-4)))
			p.rebuildAt(y)
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

// handleLiveKey 实时视图键位：焦点面板制——非焦点面板永远贴底，
// 焦点面板上翻即暂停该面板跟随，G/End 回底恢复。
// 帮助浮层打开时接管按键：Ctrl+C 仍直接退出，其余任意键只关浮层
// （实时视图无文本输入，? 不再有「空输入」门槛，恒为帮助键）。
func (m *Model) handleLiveKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.helpOverlay {
		if msg.String() == "ctrl+c" {
			m.shutdown()
			return m, tea.Quit
		}
		m.helpOverlay = false
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		return m, tea.Quit
	case "esc":
		m.shutdown()
		return m, tea.Quit
	case "?", "f1":
		m.helpOverlay = true
		return m, nil
	case "ctrl+r", "alt+r":
		return m, m.reenterPicker()
	case "tab":
		if n := len(m.livePanels); n > 0 {
			m.liveFocus = (m.liveFocus + 1) % n
		}
		return m, nil
	case "shift+tab":
		if n := len(m.livePanels); n > 0 {
			m.liveFocus = (m.liveFocus - 1 + n) % n
		}
		return m, nil
	case "1", "2", "3", "4":
		// 数字直达只认 1-4（面板上限 liveMaxPanels）；越界忽略，
		// 不回绕——多按一位数字不应跳到别的面板
		if n, err := strconv.Atoi(msg.String()); err == nil && n <= len(m.livePanels) {
			m.liveFocus = n - 1
		}
		return m, nil
	case "y":
		return m, m.copyLiveSearchCmd()
	case "up", "k", "ctrl+p", "alt+p":
		m.scrollLive(-1)
		return m, nil
	case "down", "j", "ctrl+n", "alt+n":
		m.scrollLive(1)
		return m, nil
	case "pgup":
		m.scrollLive(-m.focusPanelHeight())
		return m, nil
	case "pgdown":
		m.scrollLive(m.focusPanelHeight())
		return m, nil
	case "ctrl+d":
		m.scrollLive(max(1, m.focusPanelHeight()/2))
		return m, nil
	case "ctrl+u":
		m.scrollLive(-max(1, m.focusPanelHeight()/2))
		return m, nil
	case "G", "end":
		m.gotoLiveBottom()
		return m, nil
	case "g", "home":
		// 到顶即暂停跟随（顶部必然离开底部）；偏移 0 无须先对齐内容
		if p := m.focusPanel(); p != nil {
			p.follow = false
			p.vp.SetYOffset(0)
		}
		return m, nil
	}
	return m, nil
}

// focusPanel 当前焦点面板；面板为空（不该出现）返回 nil，调用方各自判空。
func (m *Model) focusPanel() *livePanel {
	if m.liveFocus < len(m.livePanels) {
		return m.livePanels[m.liveFocus]
	}
	return nil
}

// focusPanelHeight 焦点面板视口高（翻页/半页滚动步长），下限 1 防零步死循环。
func (m *Model) focusPanelHeight() int {
	if p := m.focusPanel(); p != nil {
		return max(1, p.vp.Height())
	}
	return 1
}

// scrollLive 滚动焦点面板并按是否贴底更新跟随标志：滚到底部自动恢复
// 跟随（与 G 对称），离开底部即暂停。非焦点面板不由此函数触及，
// 其 follow 恒真、新行 rebuild 自会贴底。
// 暂停期间新行只入缓冲不重建视口（liveLinesMsg 仅 follow 时 rebuild），
// 故先同步内容再算偏移——否则 maxY 按新缓冲计、SetYOffset 却被旧内容
// clamp，滚动会停在过期底部。
func (m *Model) scrollLive(delta int) {
	p := m.focusPanel()
	if p == nil || len(p.buf) == 0 {
		return
	}
	p.vp.SetContentLines(p.buf)
	maxY := max(0, len(p.buf)-p.vp.Height())
	y := min(max(0, p.vp.YOffset()+delta), maxY)
	p.vp.SetYOffset(y)
	p.follow = y >= maxY
}

// gotoLiveBottom 焦点面板回底并恢复跟随。容器安静（暂停后再无新行）
// 时没有 liveLinesMsg 触发 rebuild，回底必须就地完成，先对齐内容
// 同 scrollLive 的理由。
func (m *Model) gotoLiveBottom() bool {
	p := m.focusPanel()
	if p == nil {
		return false
	}
	p.follow = true
	p.vp.SetContentLines(p.buf)
	p.vp.SetYOffset(max(0, len(p.buf)-p.vp.Height()))
	return true
}

// copyLiveSearchCmd 复制焦点面板的落盘搜索命令——实时与搜索两个入口的
// 衔接：另开终端粘贴即得「边跟随边搜索」。
func (m *Model) copyLiveSearchCmd() tea.Cmd {
	p := m.focusPanel()
	if p == nil {
		return nil
	}
	cmd := "scx-rg --follow " + p.path
	if m.writeClipboard == nil {
		m.writeClipboard = writeClipboardDefault
	}
	if err := m.writeClipboard(cmd); err != nil {
		m.notice = "复制失败: " + err.Error()
		return nil
	}
	m.notice = "已复制: " + cmd
	return nil
}
