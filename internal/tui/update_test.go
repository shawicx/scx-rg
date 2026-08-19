package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

func newContentModel(t *testing.T, files map[string]string) *Model {
	t.Helper()
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{
		Root:        dir,
		Mode:        ModeContent,
		Debounce:    time.Millisecond,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

// triggerSearch 模拟防抖到期，驱动整条 cmd 链直到结束。
func triggerSearch(m *Model) {
	_, cmd := m.Update(debounceMsg{version: m.version})
	m.drain(cmd)
}

func TestStreamSearchAppendsResultsAndFollowsPreview(t *testing.T) {
	m := newContentModel(t, map[string]string{
		"a.txt": "first needle\nplain\nsecond needle\n",
	})
	m.input.SetValue("needle")
	triggerSearch(m)

	if m.searching {
		t.Fatal("流结束后 searching 应为 false")
	}
	if n := len(m.results); n != 2 {
		t.Fatalf("流式结果数 = %d, 期望 2", n)
	}
	if m.results[0].Line != 1 || m.results[1].Line != 3 {
		t.Fatalf("行号应按出现顺序: %+v", m.results)
	}
	if m.prevPath != "a.txt" {
		t.Fatalf("预览应跟随首个结果: %q", m.prevPath)
	}
}

func TestStaleStreamResultsAreDropped(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "needle\n"})
	m.version = 9
	_, _ = m.Update(resultMsg{version: 8, result: search.Result{Path: "old.txt", Line: 1}})
	if len(m.results) != 0 {
		t.Fatal("过期版本的流式结果应被丢弃")
	}
	_, _ = m.Update(streamDoneMsg{version: 8})
	if m.searching {
		t.Fatal("过期完成消息不应影响当前状态")
	}
}

func TestNewSearchResetsListAndCancelsPreviousStream(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "needle\nneedle\n"})
	m.input.SetValue("needle")
	triggerSearch(m)
	if m.cancelSearch != nil {
		t.Fatal("正常结束后 cancelSearch 应已清空")
	}

	m.input.SetValue("nee")
	m.version++
	cmd := m.runSearch()
	if cmd == nil {
		t.Fatal("新搜索应返回 cmd")
	}
	if m.searching != true {
		t.Fatal("新搜索开始时 searching 应为 true")
	}
	if len(m.results) != 0 {
		t.Fatal("新搜索开始时应清空旧结果列表")
	}
	if m.prevPath != "" {
		t.Fatal("新搜索开始时应清空预览")
	}
	m.drain(cmd)
	if len(m.results) != 2 {
		t.Fatalf("新查询流式结果 = %d, 期望 2", len(m.results))
	}
}

func TestFilesModeResultsStillWork(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "z.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, ImgProto: preview.ProtocolNone, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.input.SetValue("z")
	triggerSearch(m)
	if len(m.results) != 1 || m.results[0].Path != "z.go" {
		t.Fatalf("文件模式应返回模糊匹配结果: %+v", m.results)
	}
}
