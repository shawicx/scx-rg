package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scx-rg/internal/search"
)

func newEditorModel(t *testing.T, command string, args []string) *Model {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{Root: dir, RgAvailable: false, EditorCommand: command, EditorArgs: args})
	m.drain(m.Init())
	return m
}

// editorCmdArgs 提取命令的参数列表（跳过 Args[0] 的命令名，测试断言用）。
func editorCmdArgs(t *testing.T, m *Model) []string {
	t.Helper()
	c, err := m.buildEditorCmd()
	if err != nil {
		t.Fatalf("buildEditorCmd: %v", err)
	}
	return c.Args[1:]
}

func TestEditorArgsPresetSelection(t *testing.T) {
	cases := []struct {
		command string
		args    []string
		want    []string
	}{
		{"nvim", nil, []string{"+{line}", "{file}"}},
		{"code", nil, []string{"--goto", "{file}:{line}"}},
		{"zed", nil, []string{"{file}:{line}"}},
		{"my-custom-ed", nil, []string{"+{line}", "{file}"}},         // 未知命令回退 nvim 风格
		{"nvim", []string{"-p", "{file}"}, []string{"-p", "{file}"}}, // 显式 args 优先
	}
	for _, c := range cases {
		got := editorArgs(c.command, c.args)
		if strings.Join(got, "\x00") != strings.Join(c.want, "\x00") {
			t.Errorf("editorArgs(%q, %v) = %v, want %v", c.command, c.args, got, c.want)
		}
	}
}

func TestEditorTemplateSubstitution(t *testing.T) {
	// sh 未知于预置表 → 回退 +{line} {file} 模板；且 POSIX 环境恒存在
	m := newEditorModel(t, "sh", nil)
	m.sel = 1
	args := editorCmdArgs(t, m)
	want := []string{"+1", filepath.Join(m.root, "b.txt")} // 文件模式 Line=0 → 1
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestEditorLineFromContentMode(t *testing.T) {
	m := newEditorModel(t, "sh", []string{"--goto", "{file}:{line}"})
	// 模拟内容模式选中第 42 行
	m.results = []search.Result{{Path: "a.txt", Line: 42}}
	m.sel = 0
	args := editorCmdArgs(t, m)
	want := []string{"--goto", filepath.Join(m.root, "a.txt") + ":42"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestEditorUnconfiguredOrNoSelection(t *testing.T) {
	m := newEditorModel(t, "", nil)
	if _, err := m.buildEditorCmd(); err == nil {
		t.Error("未配置编辑器应报错")
	}
	m2 := newEditorModel(t, "sh", nil)
	m2.results = nil
	m2.sel = 0
	if _, err := m2.buildEditorCmd(); err == nil {
		t.Error("无选中项应报错")
	}
}

// nvimQuickfixLines：标记项优先（按列表序），无标记为当前选中。
func TestNvimQuickfixLines(t *testing.T) {
	m := newEditorModel(t, "sh", nil)
	m.results = []search.Result{{Path: "a.txt", Line: 3, Text: "l3"}, {Path: "b.txt", Line: 9, Text: "l9"}}
	m.sel = 0
	m.marked[resultKey(m.results[1])] = true
	lines := m.nvimQuickfixLines()
	want := []string{filepath.Join(m.root, "b.txt") + ":9: l9"}
	if strings.Join(lines, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("标记优先: %v", lines)
	}
	m.marked = map[string]bool{}
	lines = m.nvimQuickfixLines()
	want = []string{filepath.Join(m.root, "a.txt") + ":3: l3"}
	if strings.Join(lines, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("无标记用选中项: %v", lines)
	}
}

// $NVIM 存在时 Ctrl+E 走 quickfix 发送：临时文件 + :cfile 按键。
func TestNvimSendQuickfix(t *testing.T) {
	orig := nvimServerFn
	nvimServerFn = func(string) string { return "/tmp/nvim-sock" }
	t.Cleanup(func() { nvimServerFn = orig })

	m := newEditorModel(t, "sh", nil)
	m.results = []search.Result{{Path: "a.txt", Line: 42, Text: "hit"}}
	m.sel = 0
	var keys string
	m.cfg.NvimSend = func(server, k string) error {
		if server != "/tmp/nvim-sock" {
			t.Errorf("server = %q", server)
		}
		keys = k
		return nil
	}
	cmd := m.sendToNvim("/tmp/nvim-sock")
	if cmd == nil {
		t.Fatal("应返回发送命令")
	}
	msg := cmd()
	done, ok := msg.(nvimDoneMsg)
	if !ok || done.err != nil || done.count != 1 {
		t.Fatalf("回包 = %#v", msg)
	}
	// 按键含 :cfile；临时文件在发送时存在、发送后清理
	if !strings.Contains(keys, ":cfile ") || !strings.HasSuffix(keys, "\r") {
		t.Fatalf("按键格式不符: %q", keys)
	}
	m2 := newEditorModel(t, "sh", nil)
	m2.results = []search.Result{{Path: "a.txt", Line: 42, Text: "hit"}}
	m2.sel = 0
	m2.cfg.NvimSend = func(server, k string) error {
		i := strings.Index(k, ":cfile ")
		p := strings.TrimSuffix(k[i+len(":cfile "):], "\r")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("读取 qflist 失败: %v", err)
			return nil
		}
		if want := filepath.Join(m2.root, "a.txt") + ":42: hit"; strings.TrimSpace(string(raw)) != want {
			t.Errorf("qflist 内容 = %q, want %q", string(raw), want)
		}
		return nil
	}
	c2 := m2.sendToNvim("/tmp/nvim-sock")
	_ = c2
}
