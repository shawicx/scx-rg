package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/truncate"

	"scx-rg/internal/preview"
)

// View 声明式视图：内容走 frame()，alt-screen 等终端能力在此声明。
func (m *Model) View() tea.View {
	v := tea.NewView(m.frame())
	v.AltScreen = true
	return v
}

// frame 自上而下：搜索框 → [筛选栏] → [结果列表 | 预览面板] → 状态栏。
func (m *Model) frame() string {
	if m.width < 60 || m.height < 12 {
		return styleDim.Render("终端太小啦，至少需要 60x12")
	}
	parts := []string{m.headerView()}
	if m.rangeBar {
		parts = append(parts, m.rangeBarView())
	}
	switch {
	case m.helpOverlay:
		parts = append(parts, m.helpView(), m.statusView())
	case m.paletteOpen:
		parts = append(parts, m.paletteView(), m.statusView())
	case m.historyOpen:
		parts = append(parts, m.historyView(), m.statusView())
	case m.pipeOpen:
		parts = append(parts, m.pipeView(), m.statusView())
	case m.dirOpen:
		parts = append(parts, m.dirView(), m.statusView())
	case m.replaceOpen:
		parts = append(parts, m.replaceView(), m.statusView())
	case m.liveMode:
		// 实时多面板：置于 default 之前、各浮层之后——帮助/命令面板等
		// 浮层在实时模式下仍可弹出并覆盖分屏。
		parts = append(parts, m.liveView(), m.statusView())
	default:
		parts = append(parts,
			lipgloss.JoinHorizontal(lipgloss.Top, m.listView(), m.previewView()),
			m.statusView(),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) headerView() string {
	// 实时模式头部：无搜索框（输入已被分屏取代），只留标题与键位速记。
	if m.liveMode {
		name := " 实时 " + pickerKindLabel(m.pickerKind) + " "
		inner := styleAppTitle.Render(name) + styleDim.Render("  j/k 滚动 · Tab 切面板 · ? 帮助")
		inner = ansiTruncate(inner, m.frameW()-4)
		return styleInputBox.Width(m.frameW()).Render(inner)
	}
	name := " scx-rg "
	if m.picking {
		if m.pickerKind == "kubectl" {
			name = " 选择 Pod "
		} else {
			name = " 选择容器 "
		}
	} else if m.cfg.Title != "" {
		name = " " + m.cfg.Title + " "
	}
	inner := styleAppTitle.Render(name) + " " + m.input.View()
	// bubbles v2.2.0 上游缺陷：CJK 占位符的显示宽度被当作 rune 下标使用，
	// placeholderView 会把 make 零值填充的 \x00 泄漏进渲染串（宽度 0，仅污染
	// 输出与 golden 对比）。这里定向剔除；上游修复后可移除。
	inner = strings.ReplaceAll(inner, "\x00", "")
	inner = ansiTruncate(inner, m.frameW()-4)
	return styleInputBox.Width(m.frameW()).Render(inner)
}

func (m *Model) listView() string {
	if m.picking {
		return m.pickerListView()
	}
	w := m.listW - 4
	vis := m.listVisible()

	var rows []string
	if len(m.results) == 0 {
		hint := "没有匹配结果"
		switch {
		case m.searching:
			hint = "搜索中..."
		case m.searchErr != nil:
			hint = "错误: " + m.searchErr.Error()
		case m.mode == ModeContent && m.input.Value() == "":
			hint = "内容模式：输入关键词开始全文搜索"
		case m.fallbackActive:
			hint = "文件名与全文均无匹配"
		case m.mode == ModeFiles && m.input.Value() != "":
			if m.finder {
				hint = "无匹配候选"
			} else {
				hint = "文件名无匹配 / Tab 切内容模式搜全文"
			}
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

	title := stylePanelTitle.Render("结果")
	if m.fallbackActive {
		title += " " + styleBadgeContent.Render("全文")
	}
	title += styleDim.Render(fmt.Sprintf("  %d/%d", min(m.sel+1, len(m.results)), len(m.results)))
	body := title + "\n" + strings.Join(rows, "\n")
	return styleBorderActive.Width(m.listW).Render(body)
}

func (m *Model) resultRow(i, w int) string {
	r := m.results[i]
	marker := "  "
	if i == m.sel {
		marker = styleRowMarker.Render("> ")
	}
	if m.marked[resultKey(r)] {
		marker = styleRowMarker.Render("✓ ") // 标记态优先于选中指针
	}
	var body string
	if m.mode == ModeFiles && !m.fallbackActive && !m.astMode {
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
	title := stylePanelTitle.Render("预览")
	if m.finder {
		title = stylePanelTitle.Render("详情")
	}
	body := m.vp.View()
	if m.picking {
		title = stylePanelTitle.Render("详情")
		if len(m.pickerView) == 0 {
			body = stylePlaceholder.Render("选择左侧目标查看详情")
		}
		return styleBorderIdle.Width(m.prevW).Render(title + "\n" + body)
	}
	switch {
	case m.prevLoading:
		body = stylePlaceholder.Render("加载预览...")
	case m.prevPath == "" && !m.prevCustom:
		hint := "选中左侧结果后在此预览"
		if m.finder {
			hint = "选中左侧候选查看详情"
		}
		hintText := stylePlaceholder.Render(hint)
		if m.imgActive {
			// 预览被清空（新搜索/无结果）：overlay 图形只能借渲染流送出删除
			// 序列——缀在提示文本前（零宽不可见）。标志就地消费，只发一次。
			hintText = preview.KittyDeleteImage + hintText
			m.imgActive = false
		}
		body = hintText
	}
	if m.prevPath != "" {
		title += " " + styleDim.Render(m.prevPath)
		if m.prevLang != "" {
			title += styleDim.Render(" / " + m.prevLang)
		}
	}
	inner := title + "\n" + body
	return styleBorderIdle.Width(m.prevW).Render(inner)
}

// statusLine 拼状态栏单行：左侧超宽先截左，右侧提示超宽截右，
// 保证状态栏恒 1 行（帧高不变式，见 frame_width_test）。
func (m *Model) statusLine(left, right string) string {
	if lw := lipgloss.Width(left); lw > m.frameW() {
		left = ansiTruncate(left, m.frameW())
	}
	avail := m.frameW() - lipgloss.Width(left)
	if rw := lipgloss.Width(right); rw > avail {
		right = ansiTruncate(right, max(0, avail))
	}
	if pad := m.frameW() - lipgloss.Width(left) - lipgloss.Width(right); pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	return styleStatus.Width(m.frameW()).Render(left + right)
}

func (m *Model) statusView() string {
	if m.liveMode {
		return m.liveStatus()
	}
	if m.picking {
		left := styleBadgeFiles.Render("选择 " + pickerKindLabel(m.pickerKind))
		if m.pickLoading {
			left += " " + styleSearching.Render("* 抓取日志中...")
		} else if m.listLoading {
			left += " " + styleSearching.Render("* 加载列表")
		}
		left += fmt.Sprintf(" %d 项", len(m.pickerView))
		if m.searchErr != nil {
			left += " / " + styleErrText.Render(m.searchErr.Error())
		}
		if m.notice != "" {
			left += " / " + styleMatch.Render(m.notice)
		}
		right := "上下选择 / 输入过滤 / Ctrl+R 刷新 / Enter 抓取并检索 / Esc 退出"
		if m.cfg.LivePick {
			right = "上下选择 / Tab 多选(≤4) / Ctrl+R 刷新 / Enter 实时 / Esc 退出"
		}
		return m.statusLine(left, right)
	}
	var badge string
	literalOn := m.matchLiteral && (m.mode == ModeContent || m.fallbackActive)
	if m.gitLog {
		badge = styleBadgeFiles.Render("Git 历史")
	} else if m.finder {
		name := m.cfg.FinderName
		if name == "" {
			name = "finder"
		}
		badge = styleBadgeFiles.Render(name)
		if m.matchExact {
			badge += " " + styleBadgeContent.Render("精确")
		}
	} else if m.mode == ModeFiles {
		badge = styleBadgeFiles.Render("文件")
		if m.matchExact && !m.fallbackActive {
			badge += " " + styleBadgeContent.Render("精确")
		}
	} else {
		badge = styleBadgeContent.Render("内容")
	}
	if literalOn {
		badge += " " + styleBadgeContent.Render("字面")
	}
	left := badge + " "
	if m.searching {
		left += styleSearching.Render("* 搜索中") + " / "
	}
	left += fmt.Sprintf("%d 项", len(m.results))
	if m.blameOn && m.blameText != "" {
		left += " " + styleDim.Render(m.blameText)
	}
	if n := m.markedCount(); n > 0 {
		left += " " + styleBadgeContent.Render(fmt.Sprintf("已标记 %d", n))
	}
	if n := len(m.extraRoots); n > 0 {
		left += " " + styleBadgeContent.Render("+"+strconv.Itoa(n)+" 目录")
	}
	left += m.filterStatus()
	left += m.followStatus()
	if m.notice != "" {
		left += " / " + styleMatch.Render(m.notice)
	}
	if m.searchErr != nil {
		left += " / " + styleErrText.Render(m.searchErr.Error())
	}
	right := "? 帮助 / Ctrl+O 翻页复制 / Ctrl+Y 复制行 / Ctrl+T 筛选 / Ctrl+F 匹配 / Enter 选定 / Esc 清空"
	if m.finder {
		right = "? 帮助 / Ctrl+Space 标记 / Ctrl+F 匹配 / Enter 输出 / Esc 清空"
	}
	if m.pickerKind != "" {
		right = "? 帮助 / Ctrl+T 筛选 / Ctrl+R 重选" + pickerTargetWord(m.pickerKind) + " / Enter 选定 / Esc 清空"
	}
	return m.statusLine(left, right)
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
