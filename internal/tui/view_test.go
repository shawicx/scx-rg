package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
)

func TestFilesModeEmptyResultHintsContentMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("the needle text"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, ImgProto: preview.ProtocolNone, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.input.SetValue("needle")
	triggerSearch(m) // files 模式：文件名不含 needle → 无结果

	view := m.View()
	if !strings.Contains(view, "Tab") || !strings.Contains(view, "内容模式") {
		t.Fatalf("文件模式无结果时应提示切换内容模式，实际视图:\n%s", view)
	}
}
