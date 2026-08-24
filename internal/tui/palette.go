package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

// 命令面板：输入为空时按 `:` 打开，模糊过滤命令列表，Enter 执行。
// 与 `?` 帮助、`|`（M7）构成「空输入和弦族」——输入非空时这些键是普通
// 搜索字符。面板复用帮助浮层的覆盖布局（帧高不变式保持）。

// paletteItem 一条命令；run 返回执行后的续命令（nil = 仅状态变化）。
type paletteItem struct {
	title   string
	keyHint string
	run     func(m *Model) tea.Cmd
}

// paletteItems 当前模式下的命令清单（顺序即展示顺序，golden 依赖确定性）。
// M7 将追加：搜索历史 / Git 历史；M8：最近工作区。
func (m *Model) paletteItems() []paletteItem {
	items := []paletteItem{
		{title: "搜索历史", keyHint: "Ctrl+G", run: func(m *Model) tea.Cmd {
			m.historyOpen = true
			m.historySel = 0
			return nil
		}},
		{title: "打开/关闭筛选栏", keyHint: "Ctrl+T", run: func(m *Model) tea.Cmd { return m.toggleRangeBar() }},
		{title: "键位帮助", keyHint: "?", run: func(m *Model) tea.Cmd {
			m.helpOverlay = true
			return nil
		}},
		{title: "切换主题（当前 " + m.themePreset + "）", keyHint: "", run: func(m *Model) tea.Cmd {
			return m.cycleTheme()
		}},
		{title: "退出", keyHint: "Ctrl+C", run: func(m *Model) tea.Cmd {
			m.shutdown()
			m.picked = ""
			return tea.Quit
		}},
	}
	if !m.finder {
		prepend := []paletteItem{
			{title: "切换 文件/内容 模式", keyHint: "Tab", run: func(m *Model) tea.Cmd { return m.applyModeToggle() }},
			{title: "切换匹配方式", keyHint: "Ctrl+F", run: func(m *Model) tea.Cmd { return m.applyMatchToggle() }},
		}
		if !m.gitLog {
			// 用当前关键词搜「引入/删除该代码的提交」（git log -G）
			prepend = append(prepend, paletteItem{title: "Git 历史（搜索关键词的提交来源）", keyHint: "", run: func(m *Model) tea.Cmd { return m.enterGitLog() }})
		}
		items = append(prepend, items...)
	}
	return items
}

// paletteVisible 按过滤词筛出的条目（空词 = 全部）。
func (m *Model) paletteVisible() []paletteItem {
	q := strings.TrimSpace(m.paletteQuery)
	if q == "" {
		return m.paletteItems()
	}
	var out []paletteItem
	for _, it := range m.paletteItems() {
		if fm := search.Fuzzy(q, it.title+" "+it.keyHint); fm.Matched {
			out = append(out, it)
		}
	}
	return out
}

// handlePaletteKey 命令面板按键：字符/退格编辑过滤词，↑↓ 选择，
// Enter 执行并关闭，Esc 关闭，Ctrl+C 退出。
func (m *Model) handlePaletteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		m.closePalette()
		return m, nil

	case "enter":
		items := m.paletteVisible()
		if m.paletteSel >= len(items) {
			m.closePalette()
			return m, nil
		}
		run := items[m.paletteSel].run
		m.closePalette()
		return m, run(m)

	case "up", "ctrl+p":
		if m.paletteSel > 0 {
			m.paletteSel--
		}
		return m, nil

	case "down", "ctrl+n":
		if m.paletteSel < len(m.paletteVisible())-1 {
			m.paletteSel++
		}
		return m, nil

	case "backspace":
		r := []rune(m.paletteQuery)
		if len(r) > 0 {
			m.paletteQuery = string(r[:len(r)-1])
			m.paletteSel = 0
		}
		return m, nil
	}
	if msg.Text != "" && msg.Code != tea.KeyExtended {
		m.paletteQuery += msg.Text
		m.paletteSel = 0
	}
	return m, nil
}

func (m *Model) closePalette() {
	m.paletteOpen = false
	m.paletteQuery = ""
	m.paletteSel = 0
}

// cycleTheme 循环切换命名主题（default → dracula → nord → catppuccin），
// 即时生效；会话级切换，持久化由 config.toml preset 手动配置。
func (m *Model) cycleTheme() tea.Cmd {
	idx := 0
	for i, p := range presetOrder {
		if p == m.themePreset {
			idx = i
			break
		}
	}
	next := presetOrder[(idx+1)%len(presetOrder)]
	m.themePreset = next
	ApplyTheme(next, "", "", "")
	return nil
}

// applyModeToggle Tab 的共享实现（按键与命令面板共用）；gitLog 模式下
// Tab 退出回文件模式。
func (m *Model) applyModeToggle() tea.Cmd {
	if m.gitLog {
		m.gitLog = false
	}
	if m.mode == ModeFiles {
		m.mode = ModeContent
	} else {
		m.mode = ModeFiles
	}
	m.updatePlaceholder()
	m.followKeep = ""
	return m.runSearch()
}

// applyMatchToggle Ctrl+F 的共享实现：文件模式=精确(子串)/模糊；
// 内容与全文回退=字面量(-F)/正则。
func (m *Model) applyMatchToggle() tea.Cmd {
	if m.mode == ModeContent || m.fallbackActive {
		m.matchLiteral = !m.matchLiteral
	} else {
		m.matchExact = !m.matchExact
	}
	m.version++
	m.followKeep = ""
	return m.runSearch()
}

// paletteView 命令面板视图：标题 + 过滤行 + 条目列表，按 panelH 补齐
// 空行保证帧高不变式（与帮助浮层同布局策略）。
func (m *Model) paletteView() string {
	title := stylePanelTitle.Render("命令")
	if q := m.paletteQuery; q != "" {
		title += styleDim.Render("  过滤: ") + styleMatch.Render(q)
	}
	var rows []string
	for i, it := range m.paletteVisible() {
		row := "  "
		if i == m.paletteSel {
			row = styleRowMarker.Render("> ")
		}
		hint := ""
		if it.keyHint != "" {
			hint = styleDim.Render("  " + it.keyHint)
		}
		rows = append(rows, ansiTruncate(row+it.title+hint, m.frameW()-6))
	}
	if len(rows) == 0 {
		rows = append(rows, centerLine("没有匹配的命令", m.frameW()-4, stylePlaceholder))
	}
	// 内容总行数（含标题）按面板预算截断/补齐（外框占 2 行），
	// 保持「帧行数 = 终端高度」不变式（与帮助浮层同口径）
	avail := max(0, m.panelH()-2)
	lines := append([]string{title}, rows...)
	for len(lines) < avail {
		lines = append(lines, "")
	}
	if len(lines) > avail {
		lines = lines[:avail-1]
		lines = append(lines, styleDim.Render("...（终端高度不足，已截断）"))
	}
	return styleBorderIdle.Width(m.frameW()).Render(strings.Join(lines, "\n"))
}
