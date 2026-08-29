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
// 列出容器/Pod → 模糊过滤 → Enter 抓取快照（或跟随）切入既有的检索界面，
// 免去记忆与抄写容器名。全部数据源可注入以便测试。

const defaultLogTail = 100000

type (
	// pickerLoadedMsg 源列表加载完成。
	pickerLoadedMsg struct {
		sources []logs.Source
		err     error
	}
	// snapshotReadyMsg 抓取（快照/跟随启动）完成。
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

// fetchTarget 抓取选中目标：快照写入 SnapshotDir，或启动跟随进程。
func (m *Model) fetchTarget(t logs.Target) tea.Cmd {
	m.pickerName = t.Name
	tail := m.cfg.LogTail
	if tail <= 0 {
		tail = defaultLogTail
	}
	if m.followPick {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelFollow = cancel
		path := filepath.Join(m.snapshotDir, t.Kind+".log")
		follow := m.cfg.FollowLog
		if follow == nil {
			follow = func(ctx context.Context, t logs.Target, path string) error {
				return logs.Follow(ctx, t, tail, path)
			}
		}
		return func() tea.Msg {
			if err := follow(ctx, t, path); err != nil {
				return snapshotReadyMsg{err: err}
			}
			return snapshotReadyMsg{path: path}
		}
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

// handleSnapshotReady 切入检索阶段：换根目录、清空过滤词、重跑搜索；
// 跟随模式下登记 FollowFile 并启动轮询。
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
	var cmds []tea.Cmd
	cmds = append(cmds, m.runSearch())
	if m.followPick {
		m.cfg.FollowFile = msg.path
		if st, err := os.Stat(msg.path); err == nil {
			m.followSize = st.Size()
		}
		if !m.onceMode {
			cmds = append(cmds, followTick())
		}
	}
	return tea.Batch(cmds...)
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

// reenterPicker 检索阶段返回源选择器：停掉搜索与跟随进程、清空检索态并
// 重载源列表（容器可能已增减），供重新选择目标。docker/k8s 会话 Ctrl+R。
func (m *Model) reenterPicker() tea.Cmd {
	m.stopSearch()
	m.stopLive() // 实时会话停流清面板（docker/k8s 实时 → 回选择器重选）
	if m.cancelFollow != nil {
		m.cancelFollow()
		m.cancelFollow = nil
	}
	m.cfg.FollowFile = "" // 跟随轮询与实时滑窗随之停止；重选目标后再登记
	m.followKeep = ""
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

// shutdown 退出前清理：杀掉搜索 rg 与跟随进程、停实时流、落盘搜索历史。幂等。
func (m *Model) shutdown() {
	saveHistory(m.history, m.historyCap())
	m.stopSearch()
	m.stopLive()
	if m.cancelFollow != nil {
		m.cancelFollow()
		m.cancelFollow = nil
	}
}

// handlePickerKey 选择器阶段的按键：↑↓ 导航、Enter 抓取、Ctrl+R 刷新、
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
		if !m.pickLoading && m.sel < len(m.pickerView) {
			m.pickLoading = true
			m.searchErr = nil
			return m, m.fetchTarget(m.pickerView[m.sel].Target)
		}
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
	b.WriteString("\n" + styleDim.Render("Enter 抓取最近日志并检索") + "\n")
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
	name := highlightMatch(s.Target.Name, m.input.Value())
	detail := styleDim.Render(s.Detail)
	status := pickerStatusStyle(s.Status).Render(s.Status)
	row := ansiTruncate(marker+name+"  "+detail+"  "+status, w)
	if i == m.sel {
		row = selRowStyle(w).Render(row)
	}
	return row
}
