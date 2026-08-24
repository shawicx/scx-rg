package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"scx-rg/internal/search"
)

func newPipeModel(t *testing.T) (*Model, *cmdCapture) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644)
	}
	captured := &cmdCapture{}
	m := New(Config{
		Root: dir,
		PipeRun: func(cmdStr, dir, stdin string) (string, error) {
			captured.cmd, captured.dir, captured.stdin = cmdStr, dir, stdin
			return "stdout-line-1\nstdout-line-2", nil
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m, captured
}

type cmdCapture struct {
	cmd, dir, stdin string
}

func TestPipePlaceholders(t *testing.T) {
	m, _ := newPipeModel(t)
	m.results = []search.Result{{Path: "sub/a.go", Line: 42, Text: "func main() {}"}}
	m.sel = 0
	got, ok := m.pipePlaceholders("wc -l {path} && sed -n {line}p {path} # {text}")
	if !ok {
		t.Fatal("应可实例化")
	}
	want := "wc -l " + filepath.Join(m.root, "sub/a.go") + " && sed -n 42p " + filepath.Join(m.root, "sub/a.go") + " # func main() {}"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// 标记项优先喂 stdin；无标记喂当前选中项。
func TestPipeStdinMarkedPriority(t *testing.T) {
	m, _ := newPipeModel(t)
	m.results = []search.Result{{Path: "a.txt", Text: "a.txt"}, {Path: "b.txt", Text: "b.txt"}}
	m.sel = 0
	m.marked[resultKey(m.results[1])] = true
	lines := m.pipeStdinLines()
	if strings.Join(lines, "\n") != "b.txt" {
		t.Errorf("标记项应优先: %v", lines)
	}
	m.marked = map[string]bool{}
	if got := m.pipeStdinLines(); got[0] != "a.txt" {
		t.Errorf("无标记喂选中项: %v", got)
	}
}

// | 空输入触发浮层 → 输入命令 → Enter 执行 → 输出写预览。
func TestPipeOpenTypeExecute(t *testing.T) {
	m, cap0 := newPipeModel(t)
	m.results = []search.Result{{Path: "a.txt", Text: "a.txt"}}
	m.sel = 0
	// 空输入 | 打开
	if _, cmd := m.Update(tea.KeyPressMsg{Code: '|'}); cmd != nil {
		m.drain(cmd)
	}
	if !m.pipeOpen {
		t.Fatal("| 空输入应打开管道浮层")
	}
	// 输入非空时 | 是普通字符（不触发浮层）
	m2, _ := newPipeModel(t)
	m2.input.SetValue("x")
	if _, cmd := m2.Update(tea.KeyPressMsg{Code: '|'}); cmd != nil {
		m2.drain(cmd)
	}
	if m2.pipeOpen {
		t.Error("输入非空时 | 不应打开浮层")
	}
	// 输入命令并执行
	for _, r := range "grep a" {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)}); cmd != nil {
			m.drain(cmd)
		}
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	c := cap0
	if c.cmd != "grep a" {
		t.Fatalf("应执行输入的命令: %+v", c)
	}
	if c.stdin != "a.txt\n" {
		t.Errorf("stdin 应喂选中项行: %q", c.stdin)
	}
	if !strings.Contains(m.frame(), "管道输出") || !strings.Contains(m.vp.GetContent(), "stdout-line-1") {
		t.Error("输出应写回预览面板")
	}
	if m.notice != "已喂 1 行执行" {
		t.Errorf("notice = %q", m.notice)
	}
}

// Esc 关闭不清空预览；空命令与无结果有提示。
func TestPipeGuards(t *testing.T) {
	m, _ := newPipeModel(t)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: '|'}); cmd != nil {
		m.drain(cmd)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
		m.drain(cmd)
	}
	if m.pipeOpen || m.pipeInput != "" {
		t.Error("Esc 应关闭并清空输入")
	}
	// 空命令
	m.pipeOpen = true
	m.pipeInput = "  "
	if cmd := m.execPipe(); cmd != nil {
		m.drain(cmd)
	}
	if m.notice != "空命令" {
		t.Errorf("空命令提示: %q", m.notice)
	}
	// 无结果
	m2, _ := newPipeModel(t)
	m2.pipeOpen = true
	m2.pipeInput = "wc -l"
	m2.results = nil
	if cmd := m2.execPipe(); cmd != nil {
		m2.drain(cmd)
	}
	if m2.notice != "没有可喂给命令的结果" {
		t.Errorf("无结果提示: %q", m2.notice)
	}
}
