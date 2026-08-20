package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"scx-rg/internal/search"
)

// 可视化筛选栏（Ctrl+T）：时间（过去 X 分钟）与条数（最近 N 条命中）
// 两段预设 chips，光标移动即时生效，全部在客户端完成，不重新抓取。

var rangeDurPresets = []struct {
	label string
	d     time.Duration
}{
	{"实时", 30 * time.Second}, // 滑动窗口：只看最近 30 秒，跟随模式下随时间滚动
	{"全部", 0},
	{"1分钟", time.Minute},
	{"5分钟", 5 * time.Minute},
	{"15分钟", 15 * time.Minute},
	{"1小时", time.Hour},
	{"6小时", 6 * time.Hour},
	{"24小时", 24 * time.Hour},
}

var rangeCapPresets = []struct {
	label string
	n     int
}{
	{"全部", 0},
	{"100条", 100},
	{"500条", 500},
	{"5000条", 5000},
}

var (
	styleChipCursor = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)
	styleChipActive = lipgloss.NewStyle().Bold(true).Foreground(colorCyan).Padding(0, 1)
	styleChipIdle   = styleDim.Padding(0, 1)
)

// lineTimeLayouts 行首时间戳的候选格式。时间.Parse 对无时区格式按 UTC 解析，
// syslog 这类缺年份的格式解析后年份为 0，用当年补齐。
var lineTimeLayouts = []string{
	time.RFC3339, // docker/kubectl --timestamps 快照
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
	"02/Jan/2006:15:04:05 -0700", // nginx
	"Jan _2 15:04:05",            // syslog
}

// parseLineTime 解析行首时间戳；长前缀优先，避免日期前缀截住完整时间。
func parseLineTime(s string) (time.Time, bool) {
	s = strings.TrimLeft(s, " [\t")
	if len(s) < 8 {
		return time.Time{}, false
	}
	for n := 3; n >= 1; n-- {
		prefix := strings.TrimSuffix(firstNTokens(s, n), "]")
		if prefix == "" {
			continue
		}
		for _, layout := range lineTimeLayouts {
			if ts, err := time.Parse(layout, prefix); err == nil {
				if ts.Year() < 1000 {
					ts = time.Date(time.Now().Year(), ts.Month(), ts.Day(),
						ts.Hour(), ts.Minute(), ts.Second(), ts.Nanosecond(), ts.Location())
				}
				return ts, true
			}
		}
	}
	return time.Time{}, false
}

// firstNTokens 返回 s 的前 n 个空格分隔 token（保留分隔空格）；不足 n 个则返回整串。
func firstNTokens(s string, n int) string {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			count++
			if count == n {
				return s[:i]
			}
		}
	}
	return s
}

func detectResultsTs(rs []search.Result) bool {
	for _, r := range rs {
		if _, ok := parseLineTime(r.Text); ok {
			return true
		}
	}
	return false
}

// toggleRangeBar 开/关筛选栏，切换输入焦点与预览可用高度。
func (m *Model) toggleRangeBar() tea.Cmd {
	m.rangeBar = !m.rangeBar
	if m.rangeBar {
		m.rangeSel = [2]int{durPresetIndex(m.filterDur), capPresetIndex(m.filterCap)}
		m.input.Blur()
	} else {
		m.input.Focus()
	}
	m.vp.Width = max(0, m.prevW-2)
	m.vp.Height = max(0, m.panelH()-3)
	return m.followSelectionReload() // 面板高度变化后重渲染并重新定位
}

func durPresetIndex(d time.Duration) int {
	for i, p := range rangeDurPresets {
		if p.d == d {
			return i
		}
	}
	return 0
}

func capPresetIndex(n int) int {
	for i, p := range rangeCapPresets {
		if p.n == n {
			return i
		}
	}
	return 0
}

// handleRangeBarKey 筛选栏聚焦时的按键：←→ 选 chip（即时生效），
// ↑↓/Tab 切换时间/条数段，Enter/Esc/Ctrl+T 关闭。
func (m *Model) handleRangeBarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	presets := len(rangeDurPresets)
	if m.rangeSeg == 1 {
		presets = len(rangeCapPresets)
	}
	switch msg.Type {
	case tea.KeyCtrlC:
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case tea.KeyCtrlT, tea.KeyEnter, tea.KeyEsc:
		return m, m.toggleRangeBar()
	case tea.KeyUp, tea.KeyCtrlP:
		m.rangeSeg = 0
	case tea.KeyDown, tea.KeyCtrlN:
		m.rangeSeg = 1
	case tea.KeyTab:
		m.rangeSeg = 1 - m.rangeSeg
	case tea.KeyLeft:
		if m.rangeSel[m.rangeSeg] > 0 {
			m.rangeSel[m.rangeSeg]--
			return m, m.rangeChipApply()
		}
	case tea.KeyRight:
		if m.rangeSel[m.rangeSeg] < presets-1 {
			m.rangeSel[m.rangeSeg]++
			return m, m.rangeChipApply()
		}
	}
	return m, nil
}

// rangeChipApply 把光标处的预设设为生效值并重算列表；
// 跟随模式下时间筛选激活时启动实时滑动窗口的 tick 链。
func (m *Model) rangeChipApply() tea.Cmd {
	m.filterDur = rangeDurPresets[m.rangeSel[0]].d
	m.filterCap = rangeCapPresets[m.rangeSel[1]].n
	cmd := m.refilter(true)
	if m.needsLiveTick() && !m.liveTicking && !m.onceMode {
		m.liveTicking = true
		return tea.Batch(cmd, liveTick())
	}
	return cmd
}

// liveTickInterval 实时滑动窗口的重算间隔。
const liveTickInterval = time.Second

type liveTickMsg struct{}

func liveTick() tea.Cmd {
	return tea.Tick(liveTickInterval, func(time.Time) tea.Msg { return liveTickMsg{} })
}

// needsLiveTick 实时重算的激活条件：跟随中且时间筛选生效。
// 静态快照不滑动（没有新数据，滑窗只会把列表慢慢漏空）。
func (m *Model) needsLiveTick() bool {
	return m.following() && m.filterDur > 0 && m.tsOK
}

// handleLiveTick 按当前时刻重算滑动窗口：没有新日志到达，
// 超窗的旧行也会滚出列表（实时日志窗）。筛选失效则链自然终止。
func (m *Model) handleLiveTick() tea.Cmd {
	if !m.needsLiveTick() {
		m.liveTicking = false
		return nil
	}
	var next tea.Cmd
	if !m.onceMode {
		next = liveTick()
	} else {
		m.liveTicking = false
	}
	return tea.Batch(next, m.refilter(true))
}

// resultPasses 时间筛选只丢弃「带时间戳且早于范围」的行；
// 无时间戳的行（多行堆栈的续行等）保留。
func (m *Model) resultPasses(r search.Result) bool {
	if m.filterDur > 0 && m.tsOK {
		if ts, ok := parseLineTime(r.Text); ok && m.nowFunc().Sub(ts) > m.filterDur {
			return false
		}
	}
	return true
}

// refilter 从 raw 全量重算结果（chip 切换 / 同步结果到达 / 实时重算时）。
// keep 为 true 时尽量保持当前选中项；选中项被滤掉时按索引钳位，
// 实时滚动时光标停在邻近位置而不是跳回顶部。
func (m *Model) refilter(keep bool) tea.Cmd {
	oldSel := m.sel
	var keepKey string
	if keep && oldSel < len(m.results) {
		keepKey = resultKey(m.results[oldSel])
	}
	out := make([]search.Result, 0, len(m.raw))
	for _, r := range m.raw {
		if m.resultPasses(r) {
			out = append(out, r)
		}
	}
	if m.filterCap > 0 && len(out) > m.filterCap {
		out = out[len(out)-m.filterCap:]
	}
	m.results = out
	if keepKey != "" {
		m.sel = min(oldSel, max(0, len(m.results)-1))
		for i := range m.results {
			if resultKey(m.results[i]) == keepKey {
				m.sel = i
				break
			}
		}
	} else {
		m.sel = 0
	}
	m.offset = min(m.offset, max(0, len(m.results)-1))
	m.adjustOffset()
	if len(m.results) == 0 {
		m.vp.SetContent("")
		m.prevPath = ""
		return nil
	}
	return m.followSelection()
}

// trimResultsCap 条数封顶：丢弃最旧的命中，同步修正 sel/offset。
func (m *Model) trimResultsCap() {
	if m.filterCap <= 0 || len(m.results) <= m.filterCap {
		return
	}
	drop := len(m.results) - m.filterCap
	m.results = m.results[drop:]
	m.sel = max(0, m.sel-drop)
	m.offset = max(0, m.offset-drop)
}

func (m *Model) rangeBarView() string {
	rowDur := stylePanelTitle.Render("⏱ 时间")
	for i, p := range rangeDurPresets {
		rowDur += " " + renderChip(p.label, m.rangeSeg == 0 && i == m.rangeSel[0], m.filterDur == p.d)
	}
	if !m.tsOK {
		rowDur += styleDim.Render(" · 未检测到时间戳")
	}
	rowCap := stylePanelTitle.Render("⇥ 条数")
	for i, p := range rangeCapPresets {
		rowCap += " " + renderChip(p.label, m.rangeSeg == 1 && i == m.rangeSel[1], m.filterCap == p.n)
	}
	return styleStatus.Width(m.width).Render(
		ansiTruncate(" "+rowDur, m.width) + "\n" + ansiTruncate(" "+rowCap, m.width))
}

func renderChip(label string, cursor, active bool) string {
	switch {
	case cursor:
		return styleChipCursor.Render(label)
	case active:
		return styleChipActive.Render(label)
	default:
		return styleChipIdle.Render(label)
	}
}

// filterStatus 状态栏上的生效筛选摘要。
func (m *Model) filterStatus() string {
	var s string
	if m.filterDur > 0 && m.tsOK {
		s += " · ⏱ " + durLabel(m.filterDur)
	}
	if m.filterCap > 0 {
		s += " · 末" + rangeCapLabel(m.filterCap)
	}
	return s
}

func durLabel(d time.Duration) string {
	for _, p := range rangeDurPresets {
		if p.d == d {
			return p.label
		}
	}
	return d.Truncate(time.Minute).String()
}

func rangeCapLabel(n int) string {
	for _, p := range rangeCapPresets {
		if p.n == n {
			return p.label
		}
	}
	return "N条"
}
