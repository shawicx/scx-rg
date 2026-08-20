package tui

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFilesModeAutoFallsBackToContentSearch(t *testing.T) {
	m := newContentModel(t, map[string]string{"readme.md": "支持实时搜索 needle\n普通行\n"})
	m.mode = ModeFiles // 模拟用户在文件模式输入内容词
	m.input.SetValue("搜索")
	triggerSearch(m)

	if !m.fallbackActive {
		t.Fatal("文件名无命中时应自动回退全文搜索")
	}
	if n := len(m.results); n != 1 {
		t.Fatalf("回退后应显示内容命中 1 条，实际 %d", n)
	}
	if m.results[0].Path != "readme.md" || m.results[0].Line != 1 {
		t.Fatalf("回退结果应为 readme.md:1，实际 %+v", m.results[0])
	}
	if m.prevPath != "readme.md" {
		t.Fatalf("预览应跟随回退结果: %q", m.prevPath)
	}
	view := m.View()
	if !strings.Contains(view, "全文") {
		t.Fatalf("回退状态下列表应标注「全文」来源:\n%s", view)
	}
	if !strings.Contains(view, "readme.md:1") {
		t.Fatalf("回退结果应使用内容格式（路径:行号）:\n%s", view)
	}
}

func TestFallbackResetsOnNewSearch(t *testing.T) {
	m := newContentModel(t, map[string]string{
		"readme.md": "支持实时搜索\n",
		"搜索器.md":    "",
	})
	m.mode = ModeFiles
	m.input.SetValue("实时") // 只命中 readme 内容，不命中任何文件名
	triggerSearch(m)
	if !m.fallbackActive || len(m.results) != 1 {
		t.Fatalf("前置条件失败: fallback=%v results=%d", m.fallbackActive, len(m.results))
	}

	// 新查询命中文件名 → 不再回退
	m.input.SetValue("搜索器")
	m.version++
	m.drain(m.runSearch())
	if m.fallbackActive {
		t.Fatal("新搜索应重置回退状态")
	}
	if len(m.results) != 1 || m.results[0].Path != "搜索器.md" {
		t.Fatalf("文件名命中应直接显示: %+v", m.results)
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
