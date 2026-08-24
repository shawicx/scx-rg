package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/preview"
)

// 复制与外部翻页：alt-screen 全屏 TUI 里终端原生选择会把整个界面（边框、
// 双面板）一起选上，无法只选预览文本。两个应用内出口：
//   - Ctrl+O 在 less 中打开当前预览文件：临时释放终端，纯文本随意选择复制，q 返回
//   - Ctrl+Y 复制选中行到系统剪贴板（OSC 52，SSH 远程同样有效）

// pagerDoneMsg 翻页器退出，TUI 恢复。
type pagerDoneMsg struct{ err error }

// osc52Sequence 构造 OSC 52 剪贴板转义序列（terminal clipboard set）。
func osc52Sequence(s string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\x07"
}

// writeClipboard 默认实现：把 OSC 52 序列写往控制终端。走 /dev/tty 而非
// stdout，避免和 bubbletea 渲染输出混写；可注入 fake 以便测试。
func writeClipboardDefault(s string) error {
	w, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = w.WriteString(osc52Sequence(s))
	return err
}

// copySelection 复制当前选中项：日志模式复制行文本，文件模式复制绝对路径。
func (m *Model) copySelection() tea.Cmd {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil
	}
	r := m.results[m.sel]
	text := r.Text
	if !m.cfg.PickLine {
		text = filepath.Join(m.root, r.Path)
	}
	if m.writeClipboard == nil {
		m.writeClipboard = writeClipboardDefault
	}
	if err := m.writeClipboard(text); err != nil {
		m.notice = "复制失败: " + err.Error()
		return nil
	}
	m.notice = "已复制选中" + map[bool]string{true: "行", false: "路径"}[m.cfg.PickLine]
	return nil
}

// buildPagerCmd 构造「在外部翻页器打开当前预览文件」的命令；
// less 定位到选中行（+N），more 无行号定位能力仅打开。
func (m *Model) buildPagerCmd() (*exec.Cmd, error) {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil, fmt.Errorf("没有可打开的选中项")
	}
	if m.gitLog {
		return nil, fmt.Errorf("commit 无法用翻页器打开")
	}
	if m.prevKind == string(preview.KindImage) {
		return nil, fmt.Errorf("图片预览无法用翻页器打开")
	}
	r := m.results[m.sel]
	abs := filepath.Join(m.root, r.Path)
	pager := "less"
	if _, err := exec.LookPath("less"); err != nil {
		if _, err2 := exec.LookPath("more"); err2 != nil {
			return nil, fmt.Errorf("未找到 less/more")
		}
		pager = "more"
	}
	var args []string
	if pager == "less" {
		args = []string{"-R"}
		if r.Line > 0 {
			args = append(args, fmt.Sprintf("+%d", r.Line))
		}
	} else if r.Line > 0 {
		args = append(args, fmt.Sprintf("+%d", r.Line)) // more 支持 +行号
	}
	args = append(args, abs)
	return exec.Command(pager, args...), nil
}

// openInPager 释放终端运行翻页器；退出后自动恢复 TUI（tea.ExecProcess）。
func (m *Model) openInPager() tea.Cmd {
	c, err := m.buildPagerCmd()
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	m.notice = ""
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return pagerDoneMsg{err: err}
	})
}
