package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/preview"
)

// 编辑器集成：Ctrl+E 按 config.toml [editor] 模板把选中文件在用户的
// 编辑器里打开到对应行（tea.ExecProcess 暂停 TUI，编辑器退出后自动恢复，
// 与 Ctrl+O 翻页器同机制）。Enter 的 stdout 输出契约不受影响。

// editorArgsPreset 常用编辑器的默认参数模板（[editor].args 留空时套用）。
var editorArgsPreset = map[string][]string{
	"nvim":  {"+{line}", "{file}"},
	"vim":   {"+{line}", "{file}"},
	"emacs": {"+{line}", "{file}"},
	"code":  {"--goto", "{file}:{line}"},
	"zed":   {"{file}:{line}"},
}

// editorArgs 解析生效的参数模板：显式 args 优先，其次按命令名套预置，
// 未知命令回退 nvim 风格（+行号 文件）。
func editorArgs(command string, args []string) []string {
	if len(args) > 0 {
		return args
	}
	if preset, ok := editorArgsPreset[filepath.Base(command)]; ok {
		return preset
	}
	return []string{"+{line}", "{file}"}
}

// buildEditorCmd 构造「在编辑器打开当前选中项」的命令；{file} 替换为绝对
// 路径、{line} 替换为行号（文件模式为 1）。只打开当前选中项——标记项属于
// Enter 的输出语义，不与编辑动作混用。
func (m *Model) buildEditorCmd() (*exec.Cmd, error) {
	if m.cfg.EditorCommand == "" {
		return nil, errors.New("未配置编辑器（config.toml [editor]）")
	}
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil, fmt.Errorf("没有可打开的选中项")
	}
	if m.gitLog {
		return nil, fmt.Errorf("commit 无法用编辑器打开")
	}
	if m.prevKind == string(preview.KindImage) {
		return nil, fmt.Errorf("图片预览无法用编辑器打开")
	}
	r := m.results[m.sel]
	if m.finder && m.finderPath(r.Path) == "" {
		return nil, fmt.Errorf("候选不是文件路径")
	}
	bin := m.cfg.EditorCommand
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("未找到编辑器 %s", bin)
	}
	abs := r.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(m.root, abs)
	}
	args := make([]string, 0, len(m.cfg.EditorArgs)+1)
	for _, a := range editorArgs(m.cfg.EditorCommand, m.cfg.EditorArgs) {
		a = strings.ReplaceAll(a, "{file}", abs)
		a = strings.ReplaceAll(a, "{line}", strconv.Itoa(max(1, r.Line)))
		args = append(args, a)
	}
	return exec.Command(bin, args...), nil
}

// nvimServerFn 取 $NVIM（nvim --listen 启动时自设）；测试可注入。
var nvimServerFn = os.Getenv

// openInEditor 释放终端运行编辑器；退出后自动恢复 TUI。
// 检测到 $NVIM 时改为把结果集发送到已有 nvim 会话的 quickfix
// （nvim --server --remote-send :cfile），不打断当前编辑会话。
func (m *Model) openInEditor() tea.Cmd {
	if server := nvimServerFn("NVIM"); server != "" {
		return m.sendToNvim(server)
	}
	c, err := m.buildEditorCmd()
	if err != nil {
		m.notice = err.Error()
		return nil
	}
	m.notice = ""
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorDoneMsg{err: err}
	})
}

// editorDoneMsg 编辑器退出，TUI 恢复。
type editorDoneMsg struct{ err error }

// nvimDoneMsg quickfix 发送完成。
type nvimDoneMsg struct {
	count int
	err   error
}

// nvimQuickfixLines 发送目标：标记项（按列表序，过滤掉已失效的）优先，
// 无标记为当前选中项。
func (m *Model) nvimQuickfixLines() []string {
	var out []string
	if len(m.marked) > 0 {
		for _, r := range m.results {
			if !m.marked[resultKey(r)] {
				continue
			}
			abs := r.Path
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(m.root, abs)
			}
			out = append(out, fmt.Sprintf("%s:%d: %s", abs, max(1, r.Line), r.Text))
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return nil
	}
	r := m.results[m.sel]
	abs := r.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(m.root, abs)
	}
	return []string{fmt.Sprintf("%s:%d: %s", abs, max(1, r.Line), r.Text)}
}

// sendToNvim 把选中/标记结果写入临时 qflist 文件，经 --remote-send 执行
// :cfile 装入目标会话的 quickfix。
func (m *Model) sendToNvim(server string) tea.Cmd {
	lines := m.nvimQuickfixLines()
	if len(lines) == 0 {
		m.notice = "没有可发送的选中项"
		return nil
	}
	f, err := os.CreateTemp("", "scx-rg-qf-*.txt")
	if err != nil {
		m.notice = "创建临时文件失败: " + err.Error()
		return nil
	}
	path := f.Name()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		m.notice = "写入临时文件失败: " + err.Error()
		return nil
	}
	_ = f.Close()
	send := m.cfg.NvimSend
	if send == nil {
		send = func(server, keys string) error {
			c := exec.Command("nvim", "--server", server, "--remote-send", keys)
			return c.Run()
		}
	}
	count := len(lines)
	return func() tea.Msg {
		// C-\ C-N 回到 normal 模式后 :cfile 装入 quickfix
		err := send(server, "\x1c\x0e:cfile "+path+"\r")
		_ = os.Remove(path)
		return nvimDoneMsg{count: count, err: err}
	}
}

// handleNvimDone 发送结果提示。
func (m *Model) handleNvimDone(msg nvimDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.notice = "发送 nvim 失败: " + msg.err.Error()
		return nil
	}
	m.notice = fmt.Sprintf("已发送 %d 项到 nvim quickfix", msg.count)
	return nil
}
