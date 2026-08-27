package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// 多目录 workspace：命令面板「添加搜索目录」追加额外搜索根（上限 8 个），
// 主目录结果保持相对路径、额外目录结果为绝对路径。目录输入是独立浮层
// （与管道输入同模式），支持 ~ 展开；Enter 校验目录存在后生效并重跑搜索。

const maxExtraRoots = 8

// handleDirKey 目录输入浮层按键。
func (m *Model) handleDirKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		m.dirOpen = false
		m.dirInput = ""
		return m, nil

	case "enter":
		return m, m.addExtraRoot(m.dirInput)
	}
	if msg.String() == "backspace" {
		r := []rune(m.dirInput)
		if len(r) > 0 {
			m.dirInput = string(r[:len(r)-1])
		}
		return m, nil
	}
	if msg.Text != "" && msg.Code != tea.KeyExtended {
		m.dirInput += msg.Text
	}
	return m, nil
}

// addExtraRoot 校验并登记一个额外搜索根，随后重跑当前查询。
func (m *Model) addExtraRoot(input string) tea.Cmd {
	m.dirOpen = false
	p := expandHome(strings.TrimSpace(input))
	m.dirInput = ""
	if p == "" {
		return nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		m.notice = "路径解析失败: " + err.Error()
		return nil
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		m.notice = "不是有效目录: " + p
		return nil
	}
	if abs == m.root {
		m.notice = "该目录已是主搜索目录"
		return nil
	}
	for _, r := range m.extraRoots {
		if r == abs {
			m.notice = "目录已在搜索范围内"
			return nil
		}
	}
	if len(m.extraRoots) >= maxExtraRoots {
		m.notice = "额外目录已达上限 8 个"
		return nil
	}
	m.extraRoots = append(m.extraRoots, abs)
	m.version++
	m.followKeep = ""
	m.notice = "已添加目录 " + filepath.Base(abs)
	return m.runSearch()
}

// clearExtraRoots 清除全部额外目录。
func (m *Model) clearExtraRoots() tea.Cmd {
	if len(m.extraRoots) == 0 {
		return nil
	}
	m.extraRoots = nil
	m.version++
	m.followKeep = ""
	m.notice = "已清除额外目录"
	return m.runSearch()
}

// expandHome 展开开头的 ~（其余 shell 语法不支持）。
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// dirView 目录输入浮层。
func (m *Model) dirView() string {
	title := stylePanelTitle.Render("添加搜索目录") + styleDim.Render("  支持 ~ 展开")
	lines := []string{title, "", styleMatch.Render("> ") + m.dirInput, ""}
	if n := len(m.extraRoots); n > 0 {
		lines = append(lines, styleDim.Render("  当前额外目录:"))
		for _, r := range m.extraRoots {
			lines = append(lines, "  "+ansiTruncate(r, m.frameW()-6))
		}
		lines = append(lines, "")
	}
	lines = append(lines, stylePlaceholder.Render("  输入目录路径 · Enter 添加并重搜 · Esc 取消"))
	avail := max(0, m.panelH()-2)
	for len(lines) < avail {
		lines = append(lines, "")
	}
	if len(lines) > avail {
		lines = lines[:avail]
	}
	return styleBorderIdle.Width(m.frameW()).Render(strings.Join(lines, "\n"))
}
