package tui

import (
	"errors"
	"fmt"
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

// openInEditor 释放终端运行编辑器；退出后自动恢复 TUI。
func (m *Model) openInEditor() tea.Cmd {
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
