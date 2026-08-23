package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// logWindow 日志模式保留的最新命中窗口大小：命中按时间顺序流式到达，
// 超出窗口丢最旧的，保证最新日志始终可见（代码搜索仍是前 MaxResults 条）。
const logWindow = 5000

// Update 处理窗口变化、防抖到期、搜索/预览回包与按键。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.listW = m.width * 38 / 100
		m.listW = max(30, m.listW)
		if m.width-m.listW < 30 {
			m.listW = max(0, m.width-30)
		}
		m.prevW = max(0, m.frameW()-m.listW)
		m.input.SetWidth(max(10, m.width-16))

		m.resizeViewport()
		if m.prevPath != "" {
			m.prevLoading = true
			return m, m.followSelectionReload()
		}
		return m, nil

	case debounceMsg:
		if msg.version != m.version {
			return m, nil // 已有更新的输入，丢弃过期计时
		}
		return m, m.runSearch()

	case resultsMsg:
		if msg.version != m.version {
			return m, nil // 过期结果
		}
		m.searching = false
		m.searchErr = msg.err
		m.raw = msg.results
		m.tsOK = detectResultsTs(m.raw)
		cmd := m.refilter(false)
		// 文件名零命中且查询非空：自动回退全文搜索，用户不再需要记 Tab
		// （时间筛选可能清空列表，不作为回退；finder 是静态候选，无全文概念）
		if m.mode == ModeFiles && !m.finder && len(m.results) == 0 && msg.err == nil &&
			!(m.filterDur > 0 && m.tsOK) &&
			strings.TrimSpace(m.input.Value()) != "" && m.cfg.RgAvailable {
			return m, m.startFallbackStream()
		}
		return m, cmd

	case pickerLoadedMsg:
		m.listLoading = false
		if msg.err != nil {
			m.searchErr = msg.err
			return m, nil
		}
		m.searchErr = nil
		m.pickerSrcs = msg.sources
		m.pickerFilter()
		return m, nil

	case snapshotReadyMsg:
		return m, m.handleSnapshotReady(msg)

	case followTickMsg:
		return m, m.handleFollowTick()

	case liveTickMsg:
		return m, m.handleLiveTick()

	case gitFilesMsg:
		m.resizeViewport() // gitOK 翻转改变筛选栏高度，面板随之重排
		return m, m.handleGitFiles(msg)

	case pagerDoneMsg:
		if msg.err != nil {
			m.notice = "翻页器异常退出: " + msg.err.Error()
		}
		return m, nil

	case editorDoneMsg:
		if msg.err != nil {
			m.notice = "编辑器异常退出: " + msg.err.Error()
		}
		return m, nil

	case resultMsg:
		if msg.version != m.version {
			return m, nil // 过期结果
		}
		if msg.result.Err != nil {
			// 搜索本身失败（如非法正则）：终止消费并展示错误
			m.searchErr = msg.result.Err
			m.stopSearch()
			return m, nil
		}
		m.raw = append(m.raw, msg.result)
		if !m.tsOK {
			if _, ok := parseLineTime(msg.result.Text); ok {
				m.tsOK = true
			}
		}
		var cmds []tea.Cmd
		if !m.windowed && m.resultPasses(msg.result) {
			first := len(m.results) == 0
			m.results = append(m.results, msg.result)
			m.trimResultsCap()
			if first {
				cmds = append(cmds, m.followSelection()) // 首条结果到达时让预览跟上
			}
			if cmd := m.tryRestoreSelection(); cmd != nil {
				cmds = append(cmds, cmd) // 跟随刷新后恢复选中项
			}
		}
		if m.cfg.PickLine {
			// 日志模式：命中按时间顺序到达，超出窗口时丢最旧的——
			// 否则大命中量下永远只能看到最前面那批旧日志。
			// 进入窗口模式后显示列表冻结，流结束后整体重算。
			if len(m.raw) > logWindow {
				m.raw = m.raw[len(m.raw)-logWindow:]
				m.windowed = true
			}
			return m, tea.Batch(append(cmds, m.waitForResult(m.streamCh, msg.version))...)
		}
		if len(m.results) >= search.MaxResults || len(m.raw) >= search.MaxResults {
			m.stopSearch() // 封顶：杀掉 rg，剩余结果丢弃
			return m, nil
		}
		return m, tea.Batch(append(cmds, m.waitForResult(m.streamCh, msg.version))...)

	case streamDoneMsg:
		if msg.version != m.version {
			return m, nil
		}
		m.stopSearch()
		var cmd tea.Cmd
		if m.windowed {
			// 滑动窗口消费完毕：以最新窗口重建列表（按 key/索引保位）
			m.windowed = false
			cmd = m.refilter(true)
		}
		m.clearStaleKeep()
		return m, cmd

	case previewMsg:
		m.applyPreview(msg.path, msg.rendered, msg.err)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

// resizeViewport 按当前面板尺寸重设预览 viewport（窗口变化 / 筛选栏
// 行数变化后调用）。
func (m *Model) resizeViewport() {
	vpW := max(0, m.prevW-2)
	vpH := max(0, m.panelH()-3)
	if m.vp.Width() == 0 && m.vp.Height() == 0 {
		m.vp = viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
	} else {
		m.vp.SetWidth(vpW)
		m.vp.SetHeight(vpH)
	}
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.picking {
		return m.handlePickerKey(msg)
	}
	if m.rangeBar {
		return m.handleRangeBarKey(msg)
	}
	// 命令面板打开时接管按键（字符过滤 / 执行 / 关闭）
	if m.paletteOpen {
		return m.handlePaletteKey(msg)
	}
	// 帮助浮层打开时按任意键关闭（Ctrl+C 仍直接退出）
	if m.helpOverlay {
		if msg.String() == "ctrl+c" {
			m.shutdown()
			m.picked = ""
			return m, tea.Quit
		}
		m.helpOverlay = false
		return m, nil
	}
	// ? 在输入为空时打开帮助（非空时作为搜索字符）；F1 总是可用
	if msg.String() == "f1" || (msg.String() == "?" && m.input.Value() == "") {
		m.helpOverlay = true
		return m, nil
	}
	// : 在输入为空时打开命令面板（与 ? 同一空输入和弦族）
	if msg.String() == ":" && m.input.Value() == "" {
		m.paletteOpen = true
		m.paletteQuery = ""
		m.paletteSel = 0
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		m.shutdown() // 退出前杀掉可能仍在跑的 rg / 跟随进程
		m.picked = ""
		return m, tea.Quit

	case "enter":
		m.shutdown()
		m.picked = m.pickedOutput()
		return m, tea.Quit

	case "esc":
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.version++
			m.followKeep = ""
			return m, tickDebounce(m.version, m.cfg.Debounce)
		}
		if len(m.marked) > 0 { // 递进退出：先清标记，再按才退出
			m.marked = map[string]bool{}
			m.notice = "已清空标记"
			return m, nil
		}
		m.shutdown()
		return m, tea.Quit

	case "ctrl+@", "ctrl+space": // Ctrl+Space：legacy 终端发 NUL 记为 ctrl+@，kitty 协议终端记为 ctrl+space
		return m, m.toggleMark()

	case "ctrl+t":
		return m, m.toggleRangeBar()

	case "ctrl+f":
		return m, m.applyMatchToggle()

	case "ctrl+o":
		return m, m.openInPager()

	case "ctrl+e":
		return m, m.openInEditor()

	case "ctrl+y":
		return m, m.copySelection()

	case "tab":
		if m.finder {
			return m, nil // finder 模式无内容搜索概念
		}
		return m, m.applyModeToggle()

	case "up", "ctrl+p":
		if m.sel > 0 {
			m.sel--
		}
		m.adjustOffset()
		return m, m.followSelection()

	case "down", "ctrl+n":
		if m.sel < len(m.results)-1 {
			m.sel++
		}
		m.adjustOffset()
		return m, m.followSelection()

	case "pgup":
		if !m.imagePreview() {
			m.vp.ScrollUp(m.vp.Height() / 2)
		}
		return m, nil

	case "pgdown":
		if !m.imagePreview() {
			m.vp.ScrollDown(m.vp.Height() / 2)
		}
		return m, nil

	default:
		before := m.input.Value()
		newInput, cmd := m.input.Update(msg)
		m.input = newInput
		if m.input.Value() != before {
			m.version++
			m.followKeep = "" // 用户主动修改查询，保位失效
			return m, tea.Batch(cmd, tickDebounce(m.version, m.cfg.Debounce))
		}
		return m, cmd
	}
}

func (m *Model) updatePlaceholder() {
	if m.mode == ModeContent {
		if m.cfg.RgAvailable {
			m.input.Placeholder = "输入关键词，rg 全文搜索..."
		} else {
			m.input.Placeholder = "内容模式需要安装 ripgrep"
		}
	} else {
		m.input.Placeholder = "输入关键词，实时搜索..."
	}
}
