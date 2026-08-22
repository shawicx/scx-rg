package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

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
		m.input.Width = max(10, m.width-16)

		vpW := max(0, m.prevW-2)
		vpH := max(0, m.panelH()-3)
		if m.vp.Width == 0 && m.vp.Height == 0 {
			m.vp = viewport.New(vpW, vpH)
		} else {
			m.vp.Width = vpW
			m.vp.Height = vpH
		}
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
		// （时间筛选可能清空列表，不作为回退依据）
		if m.mode == ModeFiles && len(m.results) == 0 && msg.err == nil &&
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

	case pagerDoneMsg:
		if msg.err != nil {
			m.notice = "翻页器异常退出: " + msg.err.Error()
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

	case tea.KeyMsg:
		return m.handleKey(msg)

	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picking {
		return m.handlePickerKey(msg)
	}
	if m.rangeBar {
		return m.handleRangeBarKey(msg)
	}
	switch msg.Type {
	case tea.KeyCtrlC:
		m.shutdown() // 退出前杀掉可能仍在跑的 rg / 跟随进程
		m.picked = ""
		return m, tea.Quit

	case tea.KeyEnter:
		m.shutdown()
		if len(m.results) > 0 && m.sel < len(m.results) {
			if m.cfg.PickLine {
				m.picked = m.results[m.sel].Text
			} else {
				m.picked = filepath.Join(m.root, m.results[m.sel].Path)
			}
		}
		return m, tea.Quit

	case tea.KeyEsc:
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.version++
			m.followKeep = ""
			return m, tickDebounce(m.version, m.cfg.Debounce)
		}
		m.shutdown()
		return m, tea.Quit

	case tea.KeyCtrlT:
		return m, m.toggleRangeBar()

	case tea.KeyCtrlO:
		return m, m.openInPager()

	case tea.KeyCtrlY:
		return m, m.copySelection()

	case tea.KeyTab:
		if m.mode == ModeFiles {
			m.mode = ModeContent
		} else {
			m.mode = ModeFiles
		}
		m.updatePlaceholder()
		m.followKeep = ""
		return m, m.runSearch()

	case tea.KeyUp, tea.KeyCtrlP:
		if m.sel > 0 {
			m.sel--
		}
		m.adjustOffset()
		return m, m.followSelection()

	case tea.KeyDown, tea.KeyCtrlN:
		if m.sel < len(m.results)-1 {
			m.sel++
		}
		m.adjustOffset()
		return m, m.followSelection()

	case tea.KeyPgUp:
		m.vp.LineUp(m.vp.Height / 2)
		return m, nil

	case tea.KeyPgDown:
		m.vp.LineDown(m.vp.Height / 2)
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
