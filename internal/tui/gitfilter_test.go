package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

func fakeGitFiles(files []string, err error) func(context.Context, string, bool) ([]string, error) {
	return func(context.Context, string, bool) ([]string, error) { return files, err }
}

func newGitModel(t *testing.T, git func(context.Context, string, bool) ([]string, error)) *Model {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{Root: dir, RgAvailable: false, GitFiles: git})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m
}

func press(t *testing.T, m *Model, key tea.KeyPressMsg) {
	t.Helper()
	if _, cmd := m.Update(key); cmd != nil {
		m.drain(cmd)
	}
}

// Ctrl+T 打开筛选栏触发 git 探测：成功 → Git 段可见、panelH 收窄一行。
func TestGitDetectOnBarOpen(t *testing.T) {
	m := newGitModel(t, fakeGitFiles([]string{"a.txt"}, nil))
	press(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if !m.rangeBar || !m.gitOK {
		t.Fatalf("筛选栏应打开且 git 探测成功: rangeBar=%v gitOK=%v", m.rangeBar, m.gitOK)
	}
	if m.rangeBarH() != 3 || m.rangeSegs() != 3 {
		t.Errorf("git 仓库内筛选栏应为三段: barH=%d segs=%d", m.rangeBarH(), m.rangeSegs())
	}
	if !strings.Contains(m.frame(), "仅改动") {
		t.Error("Git 段应渲染 仅改动 chip")
	}
}

// 非 git 仓库：探测失败 → 第三段隐藏，两段行为不受影响。
func TestGitDetectFailureHidesSegment(t *testing.T) {
	m := newGitModel(t, fakeGitFiles(nil, errors.New("fatal: not a git repository")))
	press(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if m.gitOK || m.rangeBarH() != 2 {
		t.Errorf("非 git 仓库应为两段: gitOK=%v barH=%d", m.gitOK, m.rangeBarH())
	}
	if strings.Contains(m.frame(), "仅改动") {
		t.Error("Git 段不应出现")
	}
}

// files 模式：切到「仅改动」→ 异步文件集回包 → 重新枚举只剩集合内文件。
func TestGitFilterFilesMode(t *testing.T) {
	m := newGitModel(t, fakeGitFiles([]string{"a.txt", "b.txt"}, nil))
	press(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	// 光标移到 Git 段选「仅改动」：down down right
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.gitFilter != 1 || m.gitAllow == nil {
		t.Fatalf("gitFilter=%d allow=%v", m.gitFilter, m.gitAllow)
	}
	if len(m.results) != 2 {
		t.Fatalf("仅改动应剩 a.txt/b.txt 两条, 实际 %d: %v", len(m.results), m.results)
	}
	// 切回「全部」：恢复三条
	press(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.gitFilter != 0 || m.gitAllow != nil {
		t.Fatalf("切回全部失败: gitFilter=%d allow=%v", m.gitFilter, m.gitAllow)
	}
	if len(m.results) != 3 {
		t.Errorf("全部应恢复 3 条, 实际 %d", len(m.results))
	}
}

// content / 回退模式：Git 筛选在客户端按路径重过滤 raw 缓冲。
func TestGitFilterContentModeRefilter(t *testing.T) {
	m := newGitModel(t, fakeGitFiles([]string{"b.txt"}, nil))
	m.raw = []search.Result{{Path: "a.txt", Line: 1}, {Path: "b.txt", Line: 2}}
	m.results = m.raw
	m.gitFilter = 1
	m.gitAllow = map[string]bool{"b.txt": true}
	if cmd := m.reapplyGitFilter(); cmd != nil {
		m.drain(cmd)
	}
	if len(m.results) != 1 || m.results[0].Path != "b.txt" {
		t.Errorf("content 模式应只剩 b.txt: %v", m.results)
	}
}

// 已选 Git 筛选时拉取失败：回退「全部」并提示。
func TestGitFilterFetchErrorResets(t *testing.T) {
	m := newGitModel(t, fakeGitFiles(nil, errors.New("boom")))
	m.gitFilter = 1
	cmd := m.handleGitFiles(gitFilesMsg{err: errors.New("boom")})
	if cmd != nil {
		m.drain(cmd)
	}
	if m.gitFilter != 0 || m.gitAllow != nil || m.notice == "" {
		t.Errorf("失败应回退全部并提示: filter=%d allow=%v notice=%q", m.gitFilter, m.gitAllow, m.notice)
	}
}

// 状态栏与筛选栏显示 Git 摘要。
func TestGitFilterStatusLabels(t *testing.T) {
	m := newGitModel(t, fakeGitFiles([]string{"a.txt"}, nil))
	press(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if !strings.Contains(m.frame(), "/ 仅改动") {
		t.Error("状态栏应显示 / 仅改动")
	}
	if !strings.Contains(m.frame(), "1 文件") {
		t.Error("Git 段应显示文件数")
	}
}
