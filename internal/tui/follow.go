package tui

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/search"
)

// followInterval 跟随模式的轮询间隔（检测快照文件增长）。
const followInterval = 800 * time.Millisecond

// followTickMsg 跟随轮询到期。
type followTickMsg struct{}

func followTick() tea.Cmd {
	return tea.Tick(followInterval, func(time.Time) tea.Msg { return followTickMsg{} })
}

// following 当前是否处于跟随模式。
func (m *Model) following() bool { return m.cfg.FollowFile != "" }

// handleFollowTick 检测快照文件增长；有新内容则保位重跑当前查询。
// onceMode（--once 与测试 drain）下不续期 tick，避免同步驱动器睡眠。
func (m *Model) handleFollowTick() tea.Cmd {
	var next tea.Cmd
	if !m.onceMode {
		next = followTick()
	}
	st, err := os.Stat(m.cfg.FollowFile)
	if err != nil || st.Size() <= m.followSize {
		return next
	}
	m.followSize = st.Size()
	return tea.Batch(next, m.followRefresh())
}

// followRefresh 重跑当前查询，但尽量保持用户选中的结果项（path:line）。
func (m *Model) followRefresh() tea.Cmd {
	if m.sel < len(m.results) {
		m.followKeep = resultKey(m.results[m.sel])
	}
	return m.runSearch()
}

func resultKey(r search.Result) string {
	return r.Path + ":" + strconv.Itoa(r.Line)
}

// tryRestoreSelection 流式结果到达时尝试恢复被刷新打断的选中项。
func (m *Model) tryRestoreSelection() tea.Cmd {
	if m.followKeep == "" || m.sel >= len(m.results) {
		return nil
	}
	if resultKey(m.results[m.sel]) == m.followKeep {
		return nil // 已是目标
	}
	for i := range m.results {
		if resultKey(m.results[i]) == m.followKeep {
			m.followKeep = ""
			m.sel = i
			m.adjustOffset()
			return m.followSelection()
		}
	}
	return nil
}

// clearStaleKeep 一轮结果消费完毕后清理未命中的保位目标。
func (m *Model) clearStaleKeep() {
	m.followKeep = ""
}

// followStatus 状态栏跟随信息。
func (m *Model) followStatus() string {
	if !m.following() {
		return ""
	}
	size := m.followSize
	switch {
	case size >= 1<<20:
		return " · ⟳ 跟随 " + strconv.FormatFloat(float64(size)/(1<<20), 'f', 1, 64) + "MB"
	case size >= 1<<10:
		return " · ⟳ 跟随 " + strconv.Itoa(int(size>>10)) + "KB"
	default:
		return " · ⟳ 跟随"
	}
}
