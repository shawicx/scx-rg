package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"scx-rg/internal/search"
)

// newReplaceModel 带可注入 ast-grep 链路的模型 + 一个真实的干净 git 仓库
// 之外的普通目录（GitClean 注入恒干净，apply 走真实文件写）。
func newReplaceModel(t *testing.T) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.js"), []byte("f();\nconsole.log(1);\ng();\nconsole.log(2);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:     dir,
		GitClean: func(context.Context, string) (bool, error) { return true, nil },
		AstScan: func(ctx context.Context, root, pattern, rewrite string) ([]search.AstMatch, error) {
			return []search.AstMatch{
				{File: filepath.Join(root, "a.js"), Line: 2, Text: "console.log(1);", Replacement: "logger.debug(1);", Start: 4, End: 18},
				{File: filepath.Join(root, "a.js"), Line: 4, Text: "console.log(2);", Replacement: "logger.debug(2);", Start: 22, End: 36},
			}, nil
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m, dir
}

func withAstGrep(t *testing.T) {
	t.Helper()
	orig := astGrepAvailable
	astGrepAvailable = func() bool { return true }
	t.Cleanup(func() { astGrepAvailable = orig })
}

func typeReplace(t *testing.T, m *Model, s string) {
	t.Helper()
	for _, r := range s {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)}); cmd != nil {
			m.drain(cmd)
		}
	}
}

// 完整流：R 打开 → 模式 → Enter → 重写 → Enter 扫描 → 匹配列表 + diff 预览。
func TestReplaceScanFlow(t *testing.T) {
	withAstGrep(t)
	m, _ := newReplaceModel(t)
	if cmd := m.enterReplace(); cmd != nil {
		m.drain(cmd)
	}
	if !m.replaceOpen || m.replaceStage != 0 {
		t.Fatal("应打开第一段输入")
	}
	typeReplace(t, m, "console.log($X)")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if m.replaceStage != 1 {
		t.Fatal("Enter 应进入重写段")
	}
	typeReplace(t, m, "logger.debug($X)")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if m.replaceOpen || !m.astMode {
		t.Fatalf("应进入匹配列表: open=%v ast=%v", m.replaceOpen, m.astMode)
	}
	if len(m.results) != 2 {
		t.Fatalf("应有 2 处匹配: %v", m.results)
	}
	content := m.vp.GetContent()
	if !strings.Contains(content, "- console.log(1);") || !strings.Contains(content, "+ logger.debug(1);") {
		t.Errorf("diff 预览错误:\n%s", content)
	}
}

// y 逐条应用；a 应用全部；文件内容真实变更。
func TestReplaceApplyYAndA(t *testing.T) {
	withAstGrep(t)
	m, dir := newReplaceModel(t)
	m.drain(m.startAstScan())
	if !m.astMode || len(m.astMatches) != 2 {
		t.Fatalf("前置失败: %v", m.astMatches)
	}
	// y 应用第一条
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd != nil {
		m.drain(cmd)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "a.js"))
	if !strings.Contains(string(raw), "logger.debug(1);") || strings.Contains(string(raw), "console.log(1);") {
		t.Errorf("y 应应用第一条: %q", string(raw))
	}
	if len(m.astMatches) != 1 {
		t.Fatalf("列表应剩 1 条: %d", len(m.astMatches))
	}
	// a 应用剩余全部
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"}); cmd != nil {
		m.drain(cmd)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "a.js"))
	if strings.Contains(string(raw), "console.log(") {
		t.Errorf("a 应应用全部: %q", string(raw))
	}
	if m.astMode {
		t.Error("全部应用后应退出匹配模式")
	}
}

// 工作区不干净拒绝进入；无 ast-grep 提示。
func TestReplaceGuards(t *testing.T) {
	withAstGrep(t)
	m, _ := newReplaceModel(t)
	m.cfg.GitClean = func(context.Context, string) (bool, error) { return false, nil }
	m.replaceOpen = true
	m.replaceStage = 1
	m.replacePattern = "p"
	m.replaceRewrite = "r"
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if !strings.Contains(m.notice, "工作区不干净") {
		t.Errorf("脏工作区应拒绝: %q", m.notice)
	}
	if m.replaceOpen || m.astMode {
		t.Error("拒绝后不应进入匹配列表")
	}
	// 无 ast-grep
	astGrepAvailable = func() bool { return false }
	m2, _ := newReplaceModel(t)
	if cmd := m2.enterReplace(); cmd != nil {
		m2.drain(cmd)
	}
	if !strings.Contains(m2.notice, "未找到 ast-grep") {
		t.Errorf("缺二进制应提示: %q", m2.notice)
	}
}

// Esc 退出匹配模式，已应用的改动保留。
func TestReplaceEscExit(t *testing.T) {
	withAstGrep(t)
	m, _ := newReplaceModel(t)
	m.drain(m.startAstScan())
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
		m.drain(cmd)
	}
	if m.astMode {
		t.Error("Esc 应退出匹配模式")
	}
}
