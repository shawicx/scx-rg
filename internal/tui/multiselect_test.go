package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newMarkModel(t *testing.T, names ...string) *Model {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{Root: dir, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m
}

func pressKey(t *testing.T, m *Model, key tea.KeyType) {
	t.Helper()
	if _, cmd := m.Update(tea.KeyMsg{Type: key}); cmd != nil {
		m.drain(cmd)
	}
}

func absPath(m *Model, rel string) string { return filepath.Join(m.root, rel) }

// Ctrl+Space 标记当前行并下移；Enter 输出全部标记项（多行）。
func TestMultiSelectMarksAndOutputs(t *testing.T) {
	m := newMarkModel(t, "a.txt", "b.txt", "c.txt")
	if len(m.results) != 3 {
		t.Fatalf("前置失败：应有 3 个文件, 实际 %d", len(m.results))
	}
	first := m.results[0].Path

	pressKey(t, m, tea.KeyCtrlAt) // 标记第 1 条并下移
	if len(m.marked) != 1 || !m.marked[resultKey(m.results[0])] {
		t.Fatalf("应标记第 1 条: %v", m.marked)
	}
	if m.sel != 1 {
		t.Errorf("标记后应下移到第 2 条, sel=%d", m.sel)
	}
	if !strings.Contains(m.View(), "✓") {
		t.Error("标记行应显示 ✓")
	}
	if !strings.Contains(m.View(), "已标记 1") {
		t.Error("状态栏应显示已标记计数")
	}

	pressKey(t, m, tea.KeyCtrlAt) // 标记第 2 条（sel 停在末尾）
	if len(m.marked) != 2 {
		t.Fatalf("应标记 2 条: %v", m.marked)
	}

	// 回到第 2 条重复标记 = 取消，再重新标记
	pressKey(t, m, tea.KeyUp)
	pressKey(t, m, tea.KeyCtrlAt)
	if len(m.marked) != 1 {
		t.Fatalf("重复标记应取消: %v", m.marked)
	}
	pressKey(t, m, tea.KeyUp)
	pressKey(t, m, tea.KeyCtrlAt) // 重新标记第 2 条

	pressKey(t, m, tea.KeyEnter)

	want := absPath(m, first) + "\n" + absPath(m, m.results[1].Path)
	if m.picked != want {
		t.Errorf("picked = %q, 期望多行 %q", m.picked, want)
	}
}

// Esc 递进：输入非空先清输入，标记非空先清标记，然后才退出。
func TestMultiSelectEscProgression(t *testing.T) {
	m := newMarkModel(t, "a.txt", "b.txt")
	pressKey(t, m, tea.KeyCtrlAt)
	if len(m.marked) != 1 {
		t.Fatal("前置失败：应已标记 1 条")
	}
	pressKey(t, m, tea.KeyEsc)
	if len(m.marked) != 0 {
		t.Fatal("输入为空时 Esc 应先清空标记")
	}
	if !strings.Contains(m.View(), "已清空标记") {
		t.Error("清标记应有状态栏提示")
	}
}

// 被查询过滤掉的标记项不输出；全部失效时退回当前选中。
func TestMultiSelectFiltersStaleMarks(t *testing.T) {
	m := newMarkModel(t, "a.txt", "b.txt")
	pressKey(t, m, tea.KeyCtrlAt) // 标记 a.txt（假设列表序）

	m.input.SetValue("b")
	m.drain(m.runSearch())
	if len(m.results) != 1 || !strings.HasSuffix(m.results[0].Path, "b.txt") {
		t.Fatalf("过滤后应只剩 b.txt: %v", m.results)
	}
	if m.markedCount() != 0 {
		t.Error("被过滤的标记不应计入有效标记")
	}
	pressKey(t, m, tea.KeyEnter)
	if want := absPath(m, "b.txt"); m.picked != want {
		t.Errorf("标记全部失效时应退回当前选中: %q vs %q", m.picked, want)
	}
}

// 帧宽不变式在标记/帮助浮层下保持。
func TestMarkedFrameWidthInvariant(t *testing.T) {
	m := newMarkModel(t, "a.txt", "b.txt")
	pressKey(t, m, tea.KeyCtrlAt)
	pressKey(t, m, tea.KeyF1)
	v := m.View()
	if n := strings.Count(v, "\n") + 1; n != m.height {
		t.Errorf("帮助帧行数 %d 应等于 %d", n, m.height)
	}
	for _, l := range strings.Split(v, "\n") {
		if w := lipgloss.Width(l); w > m.frameW() {
			t.Errorf("帮助帧行宽 %d > frameW %d: %q", w, m.frameW(), l)
		}
	}
}
