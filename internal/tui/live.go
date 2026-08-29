package tui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

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

// resizeLivePanels 按 liveRows 布局重算各面板外框尺寸与内嵌视口
// （进实时 / 窗口变化时调用；仅 Update goroutine 执行）。
// 可用高度 = 终端高 − 头部搜索框 − 状态栏，按行数均分；面板内视口
// 再扣边框与标题行。完整渲染见 liveView（后续任务）。
func (m *Model) resizeLivePanels() {
	rows := liveRows(len(m.livePanels))
	if rows == nil {
		return
	}
	areaH := max(0, m.height-2)
	rowH := max(1, areaH/len(rows))
	i := 0
	for _, cnt := range rows {
		panelW := max(1, m.frameW()/cnt)
		for j := 0; j < cnt && i < len(m.livePanels); j++ {
			p := m.livePanels[i]
			p.w, p.h = panelW, rowH
			vpW := max(0, panelW-2) // 左右边框
			vpH := max(0, rowH-3)   // 上下边框 + 标题行
			if p.vp.Width() == 0 && p.vp.Height() == 0 {
				p.vp = viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
			} else {
				p.vp.SetWidth(vpW)
				p.vp.SetHeight(vpH)
			}
			p.rebuild()
			i++
		}
	}
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
