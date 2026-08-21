package tui

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

func TestCtrlYCopiesSelectedLine(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "first needle\nsecond needle\n"})
	m.input.SetValue("needle")
	triggerSearch(m)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // 选中第 2 行
	if m.results[m.sel].Line != 2 {
		t.Fatalf("前置失败: sel line=%d", m.results[m.sel].Line)
	}

	var copied string
	m.writeClipboard = func(s string) error { copied = s; return nil }
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if want := filepath.Join(m.root, "a.txt"); copied != want {
		t.Fatalf("非日志模式应复制绝对路径 %q, 得到 %q", want, copied)
	}
	if m.notice == "" {
		t.Fatal("应有复制成功提示")
	}

	// PickLine 模式（日志）复制行文本
	m.cfg.PickLine = true
	copied = ""
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if copied != "second needle" {
		t.Fatalf("日志模式应复制行文本, 得到 %q", copied)
	}
}

func TestCtrlYNoSelectionNoop(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "needle\n"})
	triggerSearch(m) // 空查询：内容模式无结果
	called := false
	m.writeClipboard = func(s string) error { called = true; return nil }
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if called {
		t.Fatal("无选中项时不应写剪贴板")
	}
}

func TestOsc52Sequence(t *testing.T) {
	got := osc52Sequence("hello")
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("hello")) + "\x07"
	if got != want {
		t.Fatalf("osc52 序列 = %q, 期望 %q", got, want)
	}
}

func TestBuildPagerCmd(t *testing.T) {
	if _, err := exec.LookPath("less"); err != nil {
		t.Skip("less 未安装")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("l1\nl2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, Mode: ModeContent, ImgProto: preview.ProtocolNone, RgAvailable: true})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.results = []search.Result{{Path: "app.log", Line: 12, Text: "hit"}}

	cmd, err := m.buildPagerCmd()
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path == "" || cmd.Args[0] != "less" {
		t.Fatalf("应使用 less: %v", cmd.Args)
	}
	joined := cmd.Args[1:]
	foundLine, foundPath := false, false
	for _, a := range joined {
		if a == "+12" {
			foundLine = true
		}
		if a == path {
			foundPath = true
		}
	}
	if !foundLine || !foundPath {
		t.Fatalf("less 参数应含 +12 定位与文件路径: %v", joined)
	}

	// 无选中项
	m.results = nil
	if _, err := m.buildPagerCmd(); err == nil {
		t.Fatal("无选中项应报错")
	}
}

func TestCtrlORequiresSelection(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "needle\n"})
	triggerSearch(m) // 空查询无结果
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if cmd != nil {
		t.Fatal("无选中项 Ctrl+O 应为空操作")
	}
}
