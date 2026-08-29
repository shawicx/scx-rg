package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"scx-rg/internal/logs"
	"scx-rg/internal/search"
)

// 源选择器：scx-rg docker / scx-rg k8s 无参数进入。
// 列出容器/Pod → 模糊过滤 → Tab 多选（实时模式 ≤4）→ Enter 分流：
// LivePick 进实时多面板，否则抓取快照切入既有的检索界面，
// 免去记忆与抄写容器名。全部数据源可注入以便测试。

const defaultLogTail = 100000

type (
	// pickerLoadedMsg 源列表加载完成。
	pickerLoadedMsg struct {
		sources []logs.Source
		err     error
	}
	// snapshotReadyMsg 抓取完成（成功携带快照路径）。
	snapshotReadyMsg struct {
		path string
		err  error
	}
)

// loadPicker 异步加载源列表。
func (m *Model) loadPicker() tea.Cmd {
	list := m.cfg.ListSources
	if list == nil {
		list = func(ctx context.Context, kind string) ([]logs.Source, error) {
			return logs.ListSources(ctx, nil, kind)
		}
	}
	return func() tea.Msg {
		srcs, err := list(context.Background(), m.pickerKind)
		return pickerLoadedMsg{sources: srcs, err: err}
	}
}

// pickerFilter 按当前输入即时过滤源列表（名称+详情模糊匹配，按得分排序）。
func (m *Model) pickerFilter() {
	q := strings.TrimSpace(m.input.Value())
	view := make([]logs.Source, 0, len(m.pickerSrcs))
	if q == "" {
		view = append(view, m.pickerSrcs...)
	} else {
		type hit struct {
			src   logs.Source
			score int
		}
		var hits []hit
		for _, s := range m.pickerSrcs {
			if fm := search.Fuzzy(q, s.Target.Name+" "+s.Detail); fm.Matched {
				hits = append(hits, hit{s, fm.Score})
			}
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		for _, h := range hits {
			view = append(view, h.src)
		}
	}
	if len(view) > search.MaxResults {
		view = view[:search.MaxResults]
	}
	m.pickerView = view
	if m.sel >= len(view) {
		m.sel = max(0, len(view)-1)
	}
	m.offset = min(m.offset, max(0, len(view)-1))
	m.pickerPreview()
}

// targetKey 选择器多选的稳定键：kind/namespace/name。
func targetKey(t logs.Target) string {
	return t.Kind + "/" + t.Namespace + "/" + t.Name
}

// fetchTarget 抓取选中目标的一次性快照写入 SnapshotDir。
// 实时跟随已由 startLive（LivePick 路径）接管，这里只剩快照路径。
func (m *Model) fetchTarget(t logs.Target) tea.Cmd {
	m.pickerName = t.Name
	tail := m.cfg.LogTail
	if tail <= 0 {
		tail = defaultLogTail
	}
	fetch := m.cfg.FetchLog
	if fetch == nil {
		fetch = func(ctx context.Context, t logs.Target) (string, error) {
			return logs.Snapshot(ctx, nil, t, tail)
		}
	}
	return func() tea.Msg {
		tmp, err := fetch(context.Background(), t)
		if err != nil {
			return snapshotReadyMsg{err: err}
		}
		path := filepath.Join(m.snapshotDir, t.Kind+".log")
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return snapshotReadyMsg{err: err}
		}
		return snapshotReadyMsg{path: path}
	}
}

// handleSnapshotReady 切入检索阶段：换根目录、清空过滤词、重跑搜索。
func (m *Model) handleSnapshotReady(msg snapshotReadyMsg) tea.Cmd {
	m.pickLoading = false
	if msg.err != nil {
		m.searchErr = msg.err // 停在选择器，可重试或换目标
		return nil
	}
	m.picking = false
	m.root = m.snapshotDir
	m.cfg.Title = pickerKindLabel(m.pickerKind) + ":" + m.pickerName
	m.cfg.PickLine = true
	m.input.SetValue("")
	m.updatePlaceholder()
	m.pickerPreview()
	return m.runSearch()
}

func pickerKindLabel(kind string) string {
	if kind == "kubectl" {
		return "k8s"
	}
	return kind
}

// pickerTargetWord 源选择器挑选的目标称谓（状态栏/帮助/命令面板用）。
func pickerTargetWord(kind string) string {
	if kind == "kubectl" {
		return "Pod"
	}
	return "容器"
}

// reenterPicker 检索阶段返回源选择器：停掉搜索与实时流、清空检索态并
// 重载源列表（容器可能已增减），供重新选择目标。docker/k8s 会话 Ctrl+R。
func (m *Model) reenterPicker() tea.Cmd {
	m.stopSearch()
	m.stopLive()          // 实时会话停流清面板（docker/k8s 实时 → 回选择器重选）
	m.cfg.FollowFile = "" // 跟随轮询与实时滑窗随之停止；重选目标后再登记
	m.followKeep = ""
	m.pickerMarks = map[string]bool{} // 上轮多选标记不跨会话残留（否则 Enter 复活旧面板）
	m.notice = ""
	m.gitLog = false
	m.rangeBar = false
	m.input.SetValue("")
	m.input.Placeholder = "输入名称过滤，实时匹配..."
	m.input.Focus() // 筛选栏可能已 Blur
	m.marked = map[string]bool{}
	m.results = nil
	m.raw = nil
	m.tsOK = false
	m.staleList = false
	m.sel, m.offset = 0, 0
	m.vp.SetContent("")
	m.prevPath = ""
	m.prevCustom = false
	m.pickerSrcs = nil
	m.pickerView = nil
	m.picking = true
	m.pickLoading = false
	m.listLoading = true
	m.searchErr = nil
	m.resizeViewport() // 筛选栏关闭后面板高度恢复
	m.pickerPreview()
	return m.loadPicker()
}

// shutdown 退出前清理：杀掉搜索 rg 进程、停实时流、落盘搜索历史。幂等。
func (m *Model) shutdown() {
	saveHistory(m.history, m.historyCap())
	m.stopSearch()
	m.stopLive() // 停流即可，无独立跟随进程（FollowPick 已退役）
}

// handlePickerKey 选择器阶段的按键：↑↓ 导航、Tab 多选标记（仅实时模式）、
// Enter 分流（LivePick 进实时多面板，否则快照检索）、Ctrl+R 刷新、
// 输入即时过滤；Esc 退出。
func (m *Model) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.pickerFilter()
			return m, nil
		}
		m.shutdown()
		return m, tea.Quit

	case "enter":
		if m.pickLoading || len(m.pickerView) == 0 || m.sel >= len(m.pickerView) {
			return m, nil
		}
		if m.cfg.LivePick {
			// 实时模式：按 pickerView 顺序收集标记项（顺序稳定，与列表一致）；
			// 无标记时退回当前选中项——单容器直达与多选同一入口
			var targets []logs.Target
			for _, s := range m.pickerView {
				if m.pickerMarks[targetKey(s.Target)] {
					targets = append(targets, s.Target)
				}
			}
			if len(targets) == 0 {
				targets = []logs.Target{m.pickerView[m.sel].Target}
			}
			m.picking = false
			m.input.Blur() // 实时视图无文本输入，回选择器时 reenterPicker 再 Focus
			m.notice = ""
			return m, m.startLive(targets)
		}
		m.pickLoading = true
		m.searchErr = nil
		return m, m.fetchTarget(m.pickerView[m.sel].Target)

	case "tab":
		// 实时模式多选标记；--snapshot（LivePick=false）禁用——快照路径
		// 单目标语义与改造前完全一致
		if !m.cfg.LivePick || m.pickLoading || m.sel >= len(m.pickerView) {
			return m, nil
		}
		k := targetKey(m.pickerView[m.sel].Target)
		if m.pickerMarks[k] {
			delete(m.pickerMarks, k)
			m.notice = ""
			return m, nil
		}
		if len(m.pickerMarks) >= liveMaxPanels {
			m.notice = "实时模式最多 4 个容器"
			return m, nil
		}
		m.pickerMarks[k] = true
		m.notice = ""
		return m, nil

	case "up", "ctrl+p", "alt+p":
		if m.sel > 0 {
			m.sel--
		}
		m.adjustOffset()
		m.pickerPreview()
		return m, nil

	case "down", "ctrl+n", "alt+n":
		if m.sel < len(m.pickerView)-1 {
			m.sel++
		}
		m.adjustOffset()
		m.pickerPreview()
		return m, nil

	case "ctrl+r", "alt+r": // 浏览器里 Ctrl+R 是刷新页面，堡垒机场景用 Alt+R
		if m.pickLoading {
			return m, nil
		}
		m.listLoading = true
		m.searchErr = nil
		return m, m.loadPicker()

	default:
		if strings.HasPrefix(msg.String(), "alt+") {
			return m, nil // Alt 组合键是命令不是文本
		}
		before := m.input.Value()
		newInput, cmd := m.input.Update(msg)
		m.input = newInput
		if m.input.Value() != before {
			m.pickerFilter() // 本地过滤，无需防抖
		}
		return m, cmd
	}
}

// pickerPreview 右侧面板显示选中源详情（同步渲染，无异步预览）。
func (m *Model) pickerPreview() {
	if len(m.pickerView) == 0 || m.sel >= len(m.pickerView) {
		m.vp.SetContent("")
		m.prevPath = ""
		m.prevCustom = false
		m.prevCustom = false
		return
	}
	s := m.pickerView[m.sel]
	var b strings.Builder
	b.WriteString(stylePanelTitle.Render(s.Target.Name) + "\n\n")
	b.WriteString(kindLine(s.Target) + "\n")
	if s.Detail != "" {
		b.WriteString("详情  " + s.Detail + "\n")
	}
	if s.Status != "" {
		b.WriteString("状态  " + pickerStatusStyle(s.Status).Render(s.Status) + "\n")
	}
	if m.cfg.LivePick {
		b.WriteString("\n" + styleDim.Render("Enter 实时日志（Tab 多选 ≤4）") + "\n")
	} else {
		b.WriteString("\n" + styleDim.Render("Enter 抓取最近日志并检索") + "\n")
	}
	b.WriteString(styleDim.Render("输入关键词过滤 / Ctrl+R 刷新 / Esc 退出") + "\n")
	m.vp.SetContent(b.String())
	m.prevPath = ""
	m.prevCustom = false // 选择器阶段不占用预览缓存
}

func kindLine(t logs.Target) string {
	if t.Kind == "kubectl" {
		line := "类型  Pod"
		if t.Namespace != "" {
			line += " / ns=" + t.Namespace
		}
		return line
	}
	return "类型  容器"
}

func pickerStatusStyle(status string) lipgloss.Style {
	if strings.Contains(status, "Up") || strings.Contains(status, "Running") {
		return styleStatusOK
	}
	return styleStatusBad
}

// pickerListView 列表面板（选择器阶段）。
func (m *Model) pickerListView() string {
	w := m.listW - 4
	vis := m.listVisible()

	var rows []string
	switch {
	case m.listLoading:
		rows = append(rows, centerLine("加载列表...", w, stylePlaceholder))
	case m.searchErr != nil:
		rows = append(rows, centerLine("错误: "+m.searchErr.Error(), w, styleErrText))
	case len(m.pickerView) == 0:
		hint := "没有可选目标"
		if m.input.Value() != "" {
			hint = "没有匹配的目标"
		}
		rows = append(rows, centerLine(hint, w, stylePlaceholder))
	default:
		for i := m.offset; i < len(m.pickerView) && i < m.offset+vis; i++ {
			rows = append(rows, m.pickerRow(i, w))
		}
	}
	for len(rows) < vis {
		rows = append(rows, "")
	}

	title := stylePanelTitle.Render(pickerKindLabel(m.pickerKind) + " 目标")
	if m.pickLoading {
		title += " " + styleSearching.Render("* 抓取中")
	}
	title += styleDim.Render(fmt.Sprintf("  %d/%d", min(m.sel+1, len(m.pickerView)), len(m.pickerView)))
	body := title + "\n" + strings.Join(rows, "\n")
	return styleBorderActive.Width(m.listW).Render(body)
}

func (m *Model) pickerRow(i, w int) string {
	s := m.pickerView[i]
	marker := "  "
	if i == m.sel {
		marker = styleRowMarker.Render("> ")
	}
	if m.pickerMarks[targetKey(s.Target)] {
		marker = styleRowMarker.Render("✓ ")
	}
	name := highlightMatch(s.Target.Name, m.input.Value())
	detail := styleDim.Render(s.Detail)
	status := pickerStatusStyle(s.Status).Render(s.Status)
	row := ansiTruncate(marker+name+"  "+detail+"  "+status, w)
	if i == m.sel {
		row = selRowStyle(w).Render(row)
	}
	return row
}
