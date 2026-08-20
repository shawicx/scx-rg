package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
)

func TestHeaderShowsCustomTitle(t *testing.T) {
	m := New(Config{Root: t.TempDir(), Title: "docker:web"})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if !strings.Contains(m.View(), "docker:web") {
		t.Fatalf("自定义标题应显示在头部:\n%s", m.View())
	}
}

func TestEnterPicksLineTextInEphemeralMode(t *testing.T) {
	m := newContentModel(t, map[string]string{"app.log": "2026-08-20 ERROR boom\n"})
	m.input.SetValue("ERROR")
	triggerSearch(m)
	m.cfg.PickLine = true // docker/日志快照等临时场景：Enter 输出行文本而非路径

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.picked != "2026-08-20 ERROR boom" {
		t.Fatalf("PickLine 模式应输出选中行文本, 得到 %q", m.picked)
	}
}

func TestWindowedPreviewScrollsToPhysicalOffset(t *testing.T) {
	// 大文件窗口化后，真实行号≠内容物理行号，滚动必须按物理行号定位
	var b strings.Builder
	for i := 1; i <= 3000; i++ {
		fmt.Fprintf(&b, "line %04d %s\n", i, strings.Repeat("x", 600))
	}
	big := filepath.Join(t.TempDir(), "big.log")
	if err := os.WriteFile(big, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(Config{Root: filepath.Dir(big), ImgProto: preview.ProtocolNone})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	ren, err := preview.Render(big, m.prevW-2, m.panelH()-3, preview.ProtocolNone, 2500)
	if err != nil {
		t.Fatal(err)
	}
	// 模拟 followSelection 已选中该文件，预览回包到达
	m.prevPath, m.prevJump, m.prevLoading = "big.log", 2500, true
	_, _ = m.Update(previewMsg{path: "big.log", rendered: ren})

	if ren.JumpOffset <= 0 {
		t.Fatalf("JumpOffset 应为正数: %d", ren.JumpOffset)
	}
	visible := m.vp.YOffset + 1 // vp 当前行号（1 起始）
	if ren.JumpOffset < visible || ren.JumpOffset > visible+m.vp.Height-1 {
		t.Fatalf("jump 物理行 %d 不在可视区 [%d, %d]",
			ren.JumpOffset, visible, visible+m.vp.Height-1)
	}
}

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
