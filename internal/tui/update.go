package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/search"
)

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
		m.prevW = max(0, m.width-m.listW)
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
		m.results = msg.results
		m.sel, m.offset = 0, 0
		// 文件名零命中且查询非空：自动回退全文搜索，用户不再需要记 Tab
		if m.mode == ModeFiles && len(m.results) == 0 && msg.err == nil &&
			strings.TrimSpace(m.input.Value()) != "" && m.cfg.RgAvailable {
			return m, m.startFallbackStream()
		}
		return m, m.followSelection()

	case resultMsg:
		if msg.version != m.version {
			return m, nil // 过期结果
		}
		first := len(m.results) == 0
		m.results = append(m.results, msg.result)
		var cmds []tea.Cmd
		if first {
			cmds = append(cmds, m.followSelection()) // 首条结果到达时让预览跟上
		}
		if len(m.results) >= search.MaxResults {
			m.stopSearch() // 封顶：杀掉 rg，剩余结果丢弃
		} else {
			cmds = append(cmds, m.waitForResult(m.streamCh, msg.version))
		}
		return m, tea.Batch(cmds...)

	case streamDoneMsg:
		if msg.version != m.version {
			return m, nil
		}
		m.stopSearch()
		return m, nil

	case previewMsg:
		if msg.path != m.prevPath {
			return m, nil // 用户已切走，丢弃
		}
		m.prevLoading = false
		if msg.err != nil {
			m.vp.SetContent(styleErrText.Render("预览失败: " + msg.err.Error()))
			return m, nil
		}
		ren := msg.rendered
		m.vp.SetContent(ren.Content)
		m.prevLines = strings.Count(ren.Content, "\n") + 1
		m.prevKind = string(ren.Kind)
		m.prevLang = ren.Lang
		if ren.JumpLine > 0 {
			m.scrollToJump(ren.JumpLine, m.prevLines)
		} else {
			m.vp.GotoTop()
		}
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
	switch msg.Type {
	case tea.KeyCtrlC:
		m.stopSearch() // 退出前杀掉可能仍在跑的 rg
		m.picked = ""
		return m, tea.Quit

	case tea.KeyEnter:
		m.stopSearch()
		if len(m.results) > 0 && m.sel < len(m.results) {
			m.picked = filepath.Join(m.root, m.results[m.sel].Path)
		}
		return m, tea.Quit

	case tea.KeyEsc:
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.version++
			return m, tickDebounce(m.version, m.cfg.Debounce)
		}
		m.stopSearch()
		return m, tea.Quit

	case tea.KeyTab:
		if m.mode == ModeFiles {
			m.mode = ModeContent
		} else {
			m.mode = ModeFiles
		}
		m.updatePlaceholder()
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
			return m, tea.Batch(cmd, tickDebounce(m.version, m.cfg.Debounce))
		}
		return m, cmd
	}
}

func (m *Model) updatePlaceholder() {
	if m.mode == ModeContent {
		if m.cfg.RgAvailable {
			m.input.Placeholder = "输入关键词，rg 全文搜索…"
		} else {
			m.input.Placeholder = "内容模式需要安装 ripgrep"
		}
	} else {
		m.input.Placeholder = "输入关键词，实时搜索…"
	}
}
