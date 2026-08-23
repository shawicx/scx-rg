package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/search"
)

func openPalette(t *testing.T, m *Model) {
	t.Helper()
	if _, cmd := m.Update(tea.KeyPressMsg{Code: ':'}); cmd != nil {
		m.drain(cmd)
	}
	if !m.paletteOpen {
		t.Fatal("前置失败：命令面板未打开")
	}
}

func typePalette(t *testing.T, m *Model, text string) {
	t.Helper()
	for _, r := range text {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)}); cmd != nil {
			m.drain(cmd)
		}
	}
}

func newPaletteModel(t *testing.T) *Model {
	t.Helper()
	m := newGitModel(t, fakeGitFiles(nil, errNotAGitRepo))
	return m
}

var errNotAGitRepo = &notAGitRepo{}

type notAGitRepo struct{}

func (*notAGitRepo) Error() string { return "fatal: not a git repository" }

// : 在空输入时打开面板；非空输入时是普通搜索字符。
func TestPaletteOpenAndClose(t *testing.T) {
	m := newPaletteModel(t)
	openPalette(t, m)
	if !strings.Contains(m.frame(), "命令") || !strings.Contains(m.frame(), "切换 文件/内容 模式") {
		t.Error("面板应展示命令列表")
	}
	// Esc 关闭
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
		m.drain(cmd)
	}
	if m.paletteOpen {
		t.Error("Esc 应关闭面板")
	}
	// 输入非空时 : 不触发
	m.input.SetValue("x")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: ':'}); cmd != nil {
		m.drain(cmd)
	}
	if m.paletteOpen {
		t.Error("输入非空时 : 不应打开面板")
	}
}

// 过滤词筛条目；Enter 执行选中命令（模式切换生效）。
func TestPaletteFilterAndExecute(t *testing.T) {
	m := newPaletteModel(t)
	if m.mode != ModeFiles {
		t.Fatal("前置失败：应从 files 模式开始")
	}
	openPalette(t, m)
	typePalette(t, m, "内容")
	vis := m.paletteVisible()
	if len(vis) != 1 || vis[0].title != "切换 文件/内容 模式" {
		t.Fatalf("过滤「内容」应只剩模式切换: %+v", vis)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if m.paletteOpen {
		t.Error("Enter 执行后面板应关闭")
	}
	if m.mode != ModeContent {
		t.Errorf("模式切换命令应生效, mode=%v", m.mode)
	}
}

// 主题循环命令：dracula → nord → catppuccin → default。
func TestPaletteThemeCycle(t *testing.T) {
	resetTheme(t)
	defer func() { ApplyTheme("", "", "", "") }()
	m := newPaletteModel(t)
	want := []string{"dracula", "nord", "catppuccin", "default"}
	for _, w := range want {
		m.cycleTheme()
		if m.themePreset != w {
			t.Errorf("循环应为 %s, 实际 %s", w, m.themePreset)
		}
	}
}

// finder 模式隐藏模式切换条目（Tab 已禁用）。
func TestPaletteFinderHidesModeToggle(t *testing.T) {
	m := New(Config{
		Candidates: []search.Candidate{{Text: "some-candidate"}},
		FinderName: "stdin",
		PickLine:   true,
	})
	m.drain(m.Init())
	openPalette(t, m)
	for _, it := range m.paletteVisible() {
		if strings.Contains(it.title, "模式") {
			t.Errorf("finder 模式不应出现模式切换条目: %s", it.title)
		}
	}
}

// 帧高不变式：面板打开时帧行数恒等于终端高度。
func TestPaletteFrameHeightInvariant(t *testing.T) {
	m := newPaletteModel(t)
	openPalette(t, m)
	if n := strings.Count(m.frame(), "\n") + 1; n != m.height {
		t.Errorf("面板帧行数 %d != 高度 %d", n, m.height)
	}
}
