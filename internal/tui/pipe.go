package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// 管道输出：输入为空时按 `|` 打开命令输入浮层（与 ? 帮助、: 面板同一
// 空输入和弦族）。命令经 sh -c 执行，占位符按当前选中项替换：
//   {path} 绝对路径 · {line} 行号 · {text} 行文本
// 标记了多选时，全部标记项的行文本从 stdin 喂给命令；否则只喂当前选中项。
// stdout+stderr 写回预览面板，不离开 TUI。

const pipeOutputCap = 200 << 10 // 预览面板承载的输出上限

// pipeDoneMsg 管道命令执行完毕。
type pipeDoneMsg struct {
	output string
	count  int // 喂给 stdin 的行数
	err    error
}

// pipePlaceholders 把命令模板按当前选中项实例化。
func (m *Model) pipePlaceholders(tpl string) (string, bool) {
	if len(m.results) == 0 || m.sel >= len(m.results) {
		return "", false
	}
	r := m.results[m.sel]
	abs := r.Path
	if !strings.HasPrefix(abs, "/") {
		abs = m.root + "/" + r.Path
	}
	replacer := strings.NewReplacer(
		"{path}", abs,
		"{line}", fmt.Sprintf("%d", max(1, r.Line)),
		"{text}", r.Text,
	)
	return replacer.Replace(tpl), true
}

// pipeStdinLines 喂给命令的行：标记项优先，无标记为当前选中项。
func (m *Model) pipeStdinLines() []string {
	if len(m.marked) > 0 {
		var out []string
		for _, r := range m.results {
			if m.marked[resultKey(r)] {
				out = append(out, r.Text)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(m.results) > 0 && m.sel < len(m.results) {
		return []string{m.results[m.sel].Text}
	}
	return nil
}

// execPipe 执行管道命令（异步），回包写预览面板。
func (m *Model) execPipe() tea.Cmd {
	tpl := m.pipeInput
	m.pipeOpen = false
	if strings.TrimSpace(tpl) == "" {
		m.notice = "空命令"
		return nil
	}
	cmdStr, ok := m.pipePlaceholders(tpl)
	if !ok {
		m.notice = "没有可喂给命令的结果"
		return nil
	}
	lines := m.pipeStdinLines()
	if len(lines) == 0 {
		m.notice = "没有可喂给命令的结果"
		return nil
	}
	m.notice = ""
	stdin := strings.Join(lines, "\n") + "\n"
	run := m.cfg.PipeRun
	if run == nil {
		run = func(cmdStr, dir, stdin string) (string, error) {
			c := exec.Command("sh", "-c", cmdStr)
			c.Dir = dir
			c.Stdin = strings.NewReader(stdin)
			var buf bytes.Buffer
			c.Stdout = &buf
			c.Stderr = &buf
			if err := c.Run(); err != nil {
				return buf.String(), err
			}
			return buf.String(), nil
		}
	}
	root := m.root
	return func() tea.Msg {
		out, err := run(cmdStr, root, stdin)
		if len(out) > pipeOutputCap {
			out = out[:pipeOutputCap] + "\n...（输出超限截断）"
		}
		return pipeDoneMsg{output: out, count: len(lines), err: err}
	}
}

// handlePipeKey 管道输入浮层按键：字符编辑，Enter 执行，Esc 关闭。
func (m *Model) handlePipeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.shutdown()
		m.picked = ""
		return m, tea.Quit

	case "esc":
		m.pipeOpen = false
		m.pipeInput = ""
		return m, nil

	case "enter":
		return m, m.execPipe()

	case "backspace":
		r := []rune(m.pipeInput)
		if len(r) > 0 {
			m.pipeInput = string(r[:len(r)-1])
		}
		return m, nil
	}
	if msg.Text != "" && msg.Code != tea.KeyExtended {
		m.pipeInput += msg.Text
	}
	return m, nil
}

// handlePipeDone 输出写回预览面板。
func (m *Model) handlePipeDone(msg pipeDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.notice = "管道命令失败: " + msg.err.Error()
	} else {
		m.notice = fmt.Sprintf("已喂 %d 行执行", msg.count)
	}
	head := stylePanelTitle.Render("管道输出") + styleDim.Render(fmt.Sprintf("  %d 行输入", msg.count)) + "\n\n"
	body := msg.output
	if strings.TrimSpace(body) == "" {
		body = stylePlaceholder.Render("（无输出）")
	}
	m.prevPath = ""
	m.prevCustom = true // 管道输出：无选中路径也要展示
	m.prevKind = ""
	m.prevLang = ""
	m.setPreviewContent(head + body)
	m.prevLines = strings.Count(head+body, "\n") + 1
	m.vp.GotoTop()
	return nil
}

// pipeView 管道输入浮层：命令行 + 实例化预览 + 占位符提示。
func (m *Model) pipeView() string {
	title := stylePanelTitle.Render("管道") + styleDim.Render("  结果行经 stdin 喂给 sh -c")
	input := styleMatch.Render("> ") + m.pipeInput
	lines := []string{title, "", input, ""}
	if eff, ok := m.pipePlaceholders(m.pipeInput); ok && strings.TrimSpace(m.pipeInput) != "" {
		lines = append(lines, styleDim.Render("  实际执行: "), "  "+ansiTruncate(eff, m.frameW()-6), "")
	} else {
		lines = append(lines, stylePlaceholder.Render("  占位符: {path} 绝对路径 · {line} 行号 · {text} 行文本"), "")
	}
	if n := len(m.pipeStdinLines()); n > 0 {
		lines = append(lines, styleDim.Render(fmt.Sprintf("  将喂 %d 行（标记项优先）· Enter 执行 · Esc 取消", n)))
	} else {
		lines = append(lines, styleDim.Render("  没有可用的结果 · Esc 取消"))
	}
	avail := max(0, m.panelH()-2)
	for len(lines) < avail {
		lines = append(lines, "")
	}
	if len(lines) > avail {
		lines = lines[:avail]
	}
	return styleBorderIdle.Width(m.frameW()).Render(strings.Join(lines, "\n"))
}
