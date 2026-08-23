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
