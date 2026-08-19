package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// View 自上而下：搜索框 → [结果列表 | 预览面板] → 状态栏。
func (m *Model) View() string {
	if m.width < 60 || m.height < 12 {
		return styleDim.Render("终端太小啦，至少需要 60×12")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		lipgloss.JoinHorizontal(lipgloss.Top, m.listView(), m.previewView()),
		m.statusView(),
	)
}

func (m *Model) headerView() string {
	title := styleAppTitle.Render(" scx-rg ")
	inner := title + " " + m.input.View()
	inner = ansiTruncate(inner, m.width-4)
	return styleInputBox.Width(m.width - 2).Render(inner)
}

func (m *Model) listView() string {
	w := m.listW - 4
	vis := m.listVisible()

	var rows []string
	if len(m.results) == 0 {
		hint := "没有匹配结果"
		switch {
		case m.searching:
			hint = "搜索中…"
		case m.searchErr != nil:
			hint = "错误: " + m.searchErr.Error()
		case m.mode == ModeContent && m.input.Value() == "":
			hint = "内容模式：输入关键词开始全文搜索"
		}
		rows = append(rows, centerLine(hint, w, stylePlaceholder))
	} else {
		for i := m.offset; i < len(m.results) && i < m.offset+vis; i++ {
			rows = append(rows, m.resultRow(i, w))
		}
	}
	for len(rows) < vis {
		rows = append(rows, "")
	}

	title := stylePanelTitle.Render("结果") +
		styleDim.Render(fmt.Sprintf("  %d/%d", min(m.sel+1, len(m.results)), len(m.results)))
	body := title + "\n" + strings.Join(rows, "\n")
	return styleBorderActive.Width(m.listW - 2).Render(body)
}

func (m *Model) resultRow(i, w int) string {
	r := m.results[i]
	marker := "  "
	if i == m.sel {
		marker = styleRowMarker.Render("❯ ")
	}
	var body string
	if m.mode == ModeFiles {
		dir, base := filepath.Split(r.Path)
		// Hits 是相对整个路径的 rune 下标，换算成 base 内的下标
		off := len([]rune(dir))
		var hits []int
		for _, h := range r.Hits {
			if h >= off {
				hits = append(hits, h-off)
			}
		}
		body = highlightRunes(base, hits) + " " + styleDim.Render(dir)
	} else {
		loc := styleDim.Render(fmt.Sprintf("%s:%d", r.Path, r.Line))
		body = loc + " " + highlightMatch(strings.TrimSpace(r.Text), m.input.Value())
	}
	row := ansiTruncate(marker+body, w)
	if i == m.sel {
		row = selRowStyle(w).Render(row)
	}
	return row
}

// highlightRunes 高亮 s 中指定 rune 下标的字符（模糊命中位置）。
func highlightRunes(s string, pos []int) string {
	if len(pos) == 0 {
		return s
	}
	set := make(map[int]bool, len(pos))
	for _, p := range pos {
		set[p] = true
	}
	var out strings.Builder
	runes := []rune(s)
	runStart := -1
	for i := 0; i <= len(runes); i++ {
		in := i < len(runes) && set[i]
		if in && runStart < 0 {
			runStart = i
		} else if !in && runStart >= 0 {
			out.WriteString(styleMatch.Render(string(runes[runStart:i])))
			runStart = -1
		}
		if i < len(runes) && !in {
			out.WriteRune(runes[i])
		}
	}
	return out.String()
}

func (m *Model) previewView() string {
	body := m.vp.View()
	if m.prevLoading {
		body = stylePlaceholder.Render("加载预览…")
	}
	title := stylePanelTitle.Render("预览")
	if m.prevPath != "" {
		title += " " + styleDim.Render(m.prevPath)
		if m.prevLang != "" {
			title += styleDim.Render(" · " + m.prevLang)
		}
	}
	inner := title + "\n" + body
	return styleBorderIdle.Width(m.prevW - 2).Render(inner)
}

func (m *Model) statusView() string {
	var badge string
	if m.mode == ModeFiles {
		badge = styleBadgeFiles.Render("文件")
	} else {
		badge = styleBadgeContent.Render("内容")
	}
	left := badge + " "
	if m.searching {
		left += styleSearching.Render("⟳ 搜索中") + " · "
	}
	left += fmt.Sprintf("%d 项", len(m.results))
	if m.searchErr != nil {
		left += " · " + styleErrText.Render(m.searchErr.Error())
	}
	right := "Tab 模式 · ↑↓ 选择 · PgUp/PgDn 滚动 · Enter 选定 · Esc 清空 · Ctrl+C 退出"
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	return styleStatus.Width(m.width).Render(left + right)
}

// highlightMatch 把 s 中出现的 q（忽略大小写）高亮成青色加粗。
func highlightMatch(s, q string) string {
	q = strings.TrimSpace(q)
	if q == "" || s == "" {
		return s
	}
	var out strings.Builder
	lowerS, lowerQ := strings.ToLower(s), strings.ToLower(q)
	for {
		idx := strings.Index(lowerS, lowerQ)
		if idx < 0 {
			out.WriteString(s)
			break
		}
		out.WriteString(s[:idx])
		out.WriteString(styleMatch.Render(s[idx : idx+len(q)]))
		s = s[idx+len(q):]
		lowerS = lowerS[idx+len(q):]
	}
	return out.String()
}

func ansiTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return truncate.String(s, uint(w))
}

func centerLine(s string, w int, st lipgloss.Style) string {
	text := st.Render(s)
	pad := (w - lipgloss.Width(text)) / 2
	if pad <= 0 {
		return text
	}
	return strings.Repeat(" ", pad) + text
}
