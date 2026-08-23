package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

func newFinderModel(t *testing.T, cands []search.Candidate, root string) *Model {
	t.Helper()
	m := New(Config{
		Root:       root,
		Candidates: cands,
		FinderName: "stdin",
		PickLine:   true,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m
}

// finder 模式：静态候选本地过滤，Enter 输出原行文本，Tab 禁用。
func TestFinderFiltersAndPicksLine(t *testing.T) {
	m := newFinderModel(t, []search.Candidate{
		{Text: "main.go", Detail: "42"},
		{Text: "readme.md"},
	}, t.TempDir())
	if !m.finder {
		t.Fatal("Candidates 非空应进入 finder 模式")
	}
	if len(m.results) != 2 {
		t.Fatalf("空查询应列出全部候选, 实际 %d", len(m.results))
	}
	if !strings.Contains(m.vp.View(), "候选行") {
		t.Fatalf("非路径候选应显示详情面板:\n%s", m.vp.View())
	}
	if !strings.Contains(m.vp.View(), "42") {
		t.Fatalf("详情面板应显示 Detail:\n%s", m.vp.View())
	}

	m.input.SetValue("main")
	m.drain(m.runSearch())
	if len(m.results) != 1 || m.results[0].Path != "main.go" {
		t.Fatalf("过滤后应只剩 main.go: %v", m.results)
	}

	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if m.picked != "main.go" {
		t.Errorf("PickLine 应输出原行文本, got %q", m.picked)
	}

	// Tab 在 finder 模式禁用
	m2 := newFinderModel(t, []search.Candidate{{Text: "x"}}, t.TempDir())
	pressKey(t, m2, tea.KeyPressMsg{Code: tea.KeyTab})
	if !m2.finder || m2.mode != ModeFiles {
		t.Error("Tab 不应切换 finder 模式")
	}
}

// 候选恰是存在的文件路径（fd | scx-rg --provider stdin）时走正常文件预览。
func TestFinderCandidateAsFilePathPreviews(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello finder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newFinderModel(t, []search.Candidate{
		{Text: filepath.Join(dir, "hello.txt")},
		{Text: "not-a-path"},
	}, dir)
	if !strings.Contains(m.vp.View(), "hello finder") {
		t.Fatalf("文件路径候选应显示文件内容:\n%s", m.vp.View())
	}
}

// finder 模式不触发文件名零命中的全文回退。
func TestFinderNoFulltextFallback(t *testing.T) {
	m := newFinderModel(t, []search.Candidate{{Text: "alpha"}}, t.TempDir())
	m.cfg.RgAvailable = true
	m.input.SetValue("zzz")
	m.drain(m.runSearch())
	if m.fallbackActive {
		t.Error("finder 模式不应回退全文搜索")
	}
	if m.searchErr != nil {
		t.Errorf("不应有搜索错误: %v", m.searchErr)
	}
}
