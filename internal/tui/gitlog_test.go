package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newGitRepoModel 建一个真实临时 git 仓库（单提交，内容含 Marker）并返回
// 基于它的模型——git 在 CI 与本机均可用，走全链路（init→log -G→show）。
func newGitRepoModel(t *testing.T) *Model {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("无 git")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc Marker() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	sh("init", "-q")
	sh("add", ".")
	sh("commit", "-qm", "add marker func")

	m := New(Config{
		Root: dir,
		GitShow: func(context.Context, string, string) (string, error) {
			return "Author: t\n\n    add marker func\n\n a.go | 3 +++\n", nil
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m
}

// enterGitLog 后：git log -G 流式出 commit 列表，详情面板显示 git show。
func TestGitLogModeFlow(t *testing.T) {
	m := newGitRepoModel(t)
	m.input.SetValue("Marker")
	m.version++
	m.drain(m.enterGitLog())
	if !m.gitLog {
		t.Fatal("应进入 gitLog 模式")
	}
	if len(m.results) != 1 {
		t.Fatalf("-G Marker 应有 1 个提交: %v", m.results)
	}
	r := m.results[0]
	if r.Path != r.Path || !strings.Contains(r.Text, "add marker func") {
		t.Errorf("commit 条目 = %+v", r)
	}
	if !strings.Contains(m.vp.GetContent(), "add marker func") {
		t.Error("详情面板应显示 commit message")
	}
	if !strings.Contains(m.frame(), "Git 历史") {
		t.Error("状态栏应显示 Git 历史 徽章")
	}
	// Enter 复制短 hash（不退出）
	clip := ""
	m.writeClipboard = func(s string) error { clip = s; return nil }
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if clip != r.Path {
		t.Errorf("Enter 应复制短 hash: %q vs %q", clip, r.Path)
	}
	if m.picked != "" {
		t.Error("gitLog 模式 Enter 不应退出")
	}
	// Tab 退出回文件模式
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}); cmd != nil {
		m.drain(cmd)
	}
	if m.gitLog {
		t.Error("Tab 应退出 gitLog")
	}
}

// gitLog 下 blame 与翻页器/编辑器防护。
func TestGitLogGuards(t *testing.T) {
	m := newGitRepoModel(t)
	m.input.SetValue("Marker")
	m.drain(m.enterGitLog())
	m.drain(m.followSelection())
	if m.blameText != "" {
		t.Error("gitLog 模式不应显示 blame")
	}
	if _, err := m.buildPagerCmd(); err == nil {
		t.Error("翻页器应拒绝 commit")
	}
	m.cfg.EditorCommand = "sh"
	if _, err := m.buildEditorCmd(); err == nil {
		t.Error("编辑器应拒绝 commit")
	}
}

// 面板入口与空查询报错。
func TestGitLogPaletteEntryAndEmptyQuery(t *testing.T) {
	m := newGitRepoModel(t)
	found := false
	for _, it := range m.paletteItems() {
		if strings.Contains(it.title, "Git 历史") {
			found = true
		}
	}
	if !found {
		t.Fatal("命令面板应有 Git 历史条目")
	}
	// 空查询：runSearch 报错进状态栏
	m.drain(m.enterGitLog())
	if m.searchErr == nil {
		t.Error("空查询应报错（git log -G 需要关键词）")
	}
}
