package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// 搜索历史：只记录「被用户实际使用过」的查询（Enter 选定 / Ctrl+E 编辑器
// / 管道执行），输入过程中的中间态不进历史。落盘 XDG state 目录
// （$XDG_STATE_HOME/scx-rg/history 或 ~/.local/state/scx-rg/history），
// JSON lines，退出时写入一次；Ctrl+G 打开浮层，Enter 回填执行，Del 删除。

const defaultHistorySize = 100

// historyStatePath 历史文件路径。
func historyStatePath() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "scx-rg", "history")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "scx-rg", "history")
}

// loadHistory 读历史文件；不存在/损坏返回空（历史是增强项，不阻断）。
func loadHistory() []string {
	path := historyStatePath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var q string
		if err := json.Unmarshal(sc.Bytes(), &q); err != nil || q == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

// saveHistory 把历史写回（截断到 cap；目录不存在则创建）。
func saveHistory(queries []string, capN int) {
	path := historyStatePath()
	if path == "" {
		return
	}
	if capN <= 0 {
		capN = defaultHistorySize
	}
	if len(queries) > capN {
		queries = queries[:capN]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, q := range queries {
		b, err := json.Marshal(q)
		if err != nil {
			continue
		}
		_, _ = w.Write(append(b, '\n'))
	}
	_ = w.Flush()
}

// recordQuery 记录一条已使用的查询：去首尾空白、与最近一条相同则不重复，
// 已存在的旧记录上移到顶部，超限截断。
func (m *Model) recordQuery(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	if n := len(m.history); n > 0 && m.history[n-1] == q {
		return // 连续使用同一条不重复记录（历史按新→旧排列，新条目在尾部）
	}
	// 去掉旧位置的同名条目
	filtered := make([]string, 0, len(m.history)+1)
	for _, h := range m.history {
		if h != q {
			filtered = append(filtered, h)
		}
	}
	m.history = append(filtered, q)
	if capN := m.historyCap(); capN > 0 && len(m.history) > capN {
		m.history = m.history[len(m.history)-capN:]
	}
}

func (m *Model) historyCap() int {
	if m.cfg.HistorySize > 0 {
		return m.cfg.HistorySize
	}
	return defaultHistorySize
}

// visibleHistory 浮层展示顺序：新→旧（最新在顶部）。
func (m *Model) visibleHistory() []string {
	out := make([]string, len(m.history))
	for i, q := range m.history {
		out[len(out)-1-i] = q
	}
	return out
}

// handleHistoryKey 历史浮层按键：↑↓ 选择，Enter 回填执行，
// Del 删除当前条，Esc 关闭。
func (m *Model) handleHistoryKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	vis := m.visibleHistory()
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		m.historyOpen = false
		m.historySel = 0
		return m, nil

	case "enter":
		if m.historySel >= len(vis) {
			m.historyOpen = false
			return m, nil
		}
		q := vis[m.historySel]
		m.historyOpen = false
		m.historySel = 0
		m.input.SetValue(q)
		m.version++
		m.followKeep = ""
		return m, m.runSearch()

	case "up", "ctrl+p":
		if m.historySel > 0 {
			m.historySel--
		}
		return m, nil

	case "down", "ctrl+n":
		if m.historySel < len(vis)-1 {
			m.historySel++
		}
		return m, nil

	case "delete", "backspace":
		if m.historySel < len(vis) {
			idx := len(vis) - 1 - m.historySel // 浮层序（新→旧）换算到存储序（旧→新）
			m.history = append(m.history[:idx], m.history[idx+1:]...)
			if m.historySel >= len(m.visibleHistory()) && m.historySel > 0 {
				m.historySel--
			}
		}
		return m, nil
	}
	return m, nil
}

// historyView 历史浮层：与命令面板同布局策略（帧高不变式保持）。
func (m *Model) historyView() string {
	title := stylePanelTitle.Render("搜索历史")
	if n := len(m.history); n > 0 {
		title += styleDim.Render(fmt.Sprintf("  %d 条（最新在前 · Del 删除）", n))
	}
	var rows []string
	for i, q := range m.visibleHistory() {
		row := "  "
		if i == m.historySel {
			row = styleRowMarker.Render("> ")
		}
		rows = append(rows, ansiTruncate(row+styleMatch.Render(q), m.frameW()-6))
	}
	if len(rows) == 0 {
		rows = append(rows, centerLine("暂无历史（选定结果时自动记录）", m.frameW()-4, stylePlaceholder))
	}
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
