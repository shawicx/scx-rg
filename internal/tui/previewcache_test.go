package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
)

// A→B→A 切选：第二次访问 A 应命中渲染缓存，不再调用 renderFile。
func TestPreviewCacheSkipsRerenderOnRevisit(t *testing.T) {
	m := newContentModel(t, map[string]string{
		"alpha.txt": "alpha line one\n",
		"bravo.txt": "bravo line one\n",
	})
	m.input.SetValue("line")
	renders := 0
	m.renderFile = func(path string, cols, rows int, proto preview.Protocol, jump int, query string) (preview.Rendered, error) {
		renders++
		return preview.Render(path, cols, rows, proto, jump, query)
	}
	triggerSearch(m) // 搜索完成，预览跟随第一条结果（第 1 次渲染）

	if len(m.results) != 2 {
		t.Fatalf("应有 2 条结果，实际 %d", len(m.results))
	}
	if renders != 1 {
		t.Fatalf("初始渲染次数 = %d, 期望 1", renders)
	}
	// rg 输出顺序不保证字母序，按实际选中项断言
	word := func(path string) string { return strings.TrimSuffix(path, ".txt") }
	first := m.results[m.sel].Path
	if !strings.Contains(m.vp.View(), word(first)) {
		t.Fatalf("预览应显示 %s:\n%s", first, m.vp.View())
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.drain(cmd) // 切到另一条（第 2 次渲染）
	if renders != 2 {
		t.Fatalf("切换后渲染次数 = %d, 期望 2", renders)
	}
	second := m.results[m.sel].Path
	if second == first || !strings.Contains(m.vp.View(), word(second)) {
		t.Fatalf("预览应显示 %s:\n%s", second, m.vp.View())
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.drain(cmd) // 切回首条：缓存命中，渲染次数不变
	if renders != 2 {
		t.Fatalf("回访应命中缓存，渲染次数 = %d, 期望仍为 2", renders)
	}
	if !strings.Contains(m.vp.View(), word(first)) {
		t.Fatalf("缓存命中的预览应显示 %s:\n%s", first, m.vp.View())
	}
	if m.prevLoading {
		t.Error("缓存命中后不应处于 loading 状态")
	}
}
