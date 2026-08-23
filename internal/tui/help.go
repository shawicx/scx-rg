package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// helpGroup 一组键位：标题 + {键, 说明} 行。
type helpGroup struct {
	title string
	rows  [][2]string
}

// helpGroups 按当前模式裁剪的完整键位表。
func (m *Model) helpGroups() []helpGroup {
	groups := []helpGroup{{
		title: "搜索",
		rows: [][2]string{
			{"输入", "实时搜索（防抖）"},
			{"Esc", "清空输入 → 清空标记 → 退出"},
		},
	}}
	if m.finder {
		groups = append(groups, helpGroup{
			title: "模式",
			rows: [][2]string{
				{"Ctrl+F", "精确 / 模糊匹配切换"},
				{"--", "Tab 切模式已禁用（静态候选）"},
			},
		})
	} else {
		groups = append(groups, helpGroup{
			title: "模式",
			rows: [][2]string{
				{"Tab", "文件 ⇄ 内容 模式切换"},
				{"Ctrl+F", "精确/模糊（文件）· 字面/正则（内容）"},
			},
		})
	}
	groups = append(groups,
		helpGroup{
			title: "列表",
			rows: [][2]string{
				{"↑ ↓", "移动选中项"},
				{"Ctrl+Space", "标记 / 取消当前行（多选）"},
				{"Enter", "输出选中项；有标记则输出全部标记项"},
			},
		},
		helpGroup{
			title: "预览",
			rows: [][2]string{
				{"PgUp PgDn", "滚动预览（图片预览除外）"},
				{"Ctrl+Y", "复制当前预览内容"},
				{"Ctrl+O", "外部翻页器打开（翻页复制）"},
			},
		},
	)
	if m.following() || m.mode == ModeContent {
		groups = append(groups, helpGroup{
			title: "筛选（日志/跟随）",
			rows: [][2]string{
				{"Ctrl+T", "打开 / 关闭结果筛选栏"},
				{"↑ ↓ Tab", "筛选栏内切段移动"},
			},
		})
	}
	groups = append(groups, helpGroup{
		title: "其他",
		rows: [][2]string{
			{"?", "本帮助（输入为空时）· F1 任何时候"},
			{"Ctrl+C", "退出"},
		},
	})
	return groups
}

// helpView 渲染帮助浮层：分组两列排布，占满主区域。
func (m *Model) helpView() string {
	groups := m.helpGroups()

	keyW := 0
	for _, g := range groups {
		for _, r := range g.rows {
			keyW = max(keyW, lipgloss.Width(r[0]))
		}
	}
	renderGroup := func(g helpGroup) string {
		var sb strings.Builder
		sb.WriteString(stylePanelTitle.Render(g.title) + "\n")
		for _, r := range g.rows {
			key := r[0] + strings.Repeat(" ", keyW-lipgloss.Width(r[0]))
			sb.WriteString(styleRowMarker.Render(key) + "  " + styleDim.Render(r[1]) + "\n")
		}
		return sb.String()
	}
	renderGroups := func(gs []helpGroup) string {
		return lipgloss.JoinVertical(lipgloss.Left, mapGroups(gs, renderGroup)...)
	}
	// 两列放得下才并排；窄终端退回单列（右列被整列截掉比单列更糟）
	innerW := max(0, m.frameW()-m.listW-4)
	half := (len(groups) + 1) / 2
	var body string
	gap := strings.Repeat(" ", 4)
	colA, colB := renderGroups(groups[:half]), renderGroups(groups[half:])
	if widestLine(colA)+lipgloss.Width(gap)+widestLine(colB) <= innerW {
		body = lipgloss.JoinHorizontal(lipgloss.Top, colA, gap, colB)
	} else {
		body = renderGroups(groups) // 单列：全部组竖排
	}
	inner := stylePanelTitle.Render("键位帮助") + styleDim.Render("  （按任意键返回）") + "\n\n" + body
	// 内容按面板内宽截断（Width() 会对超宽行折行、撑爆帧高），
	// 再撑满主区域高度（小终端截断），保持「帧行数 = 终端高度」不变式
	lines := strings.Split(inner, "\n")
	for i, l := range lines {
		lines[i] = ansiTruncate(l, innerW)
	}
	avail := max(0, m.panelH()-2)
	if len(lines) > avail {
		lines = lines[:avail-1]
		lines = append(lines, styleDim.Render("...（终端高度不足，已截断）"))
	}
	for len(lines) < avail {
		lines = append(lines, "")
	}
	return styleBorderIdle.Render(strings.Join(lines, "\n"))
}

// widestLine 多行文本中最宽行的显示宽度。
func widestLine(s string) int {
	w := 0
	for _, l := range strings.Split(s, "\n") {
		w = max(w, lipgloss.Width(l))
	}
	return w
}

func mapGroups(gs []helpGroup, f func(helpGroup) string) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = f(g)
	}
	return out
}

// markedCount 当前列表中仍然有效的标记数（用于状态栏展示）。
func (m *Model) markedCount() int {
	n := 0
	for _, r := range m.results {
		if m.marked[resultKey(r)] {
			n++
		}
	}
	return n
}
