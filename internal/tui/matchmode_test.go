package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
)

// Ctrl+F 在文件模式切换精确（子串）/ 模糊（子序列）匹配，状态栏显示当前模式。
func TestCtrlFTogglesExactMatch(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"my-great-model.go", "great.txt", "agrtz.log"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{Root: dir, ImgProto: preview.ProtocolNone, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())

	m.input.SetValue("grt")
	triggerSearch(m)
	if len(m.results) != 3 {
		t.Fatalf("模糊模式应命中 3 个（紧凑子序列），得到 %d: %v", len(m.results), m.results)
	}
	if strings.Contains(m.statusView(), "精确") {
		t.Fatal("默认应是模糊模式，状态栏不应显示精确")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m.drain(cmd)
	if !m.matchExact {
		t.Fatal("Ctrl+F 后应处于精确模式")
	}
	if len(m.results) != 1 || m.results[0].Path != "agrtz.log" {
		t.Fatalf("精确模式只应保留子串命中，得到 %v", m.results)
	}
	if !strings.Contains(m.statusView(), "精确") {
		t.Fatalf("状态栏应显示精确徽章:\n%s", m.statusView())
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m.drain(cmd)
	if m.matchExact || len(m.results) != 3 {
		t.Fatalf("再次 Ctrl+F 应切回模糊并恢复结果，matchExact=%v results=%d", m.matchExact, len(m.results))
	}
}

// 非法正则自动按字面量兜底（不报错、不改用户偏好）；Ctrl+F 仍是手动粘性切换。
func TestInvalidRegexAutoFallsBackToLiteral(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.log": "log.error( here\nlog.warn there\n"})

	// 合法正则：按正则解析，命中两行
	m.input.SetValue(`log\.(error|warn)`)
	triggerSearch(m)
	if len(m.results) != 2 {
		t.Fatalf("合法正则应命中两行，得到 %v", m.results)
	}
	if m.matchLiteral {
		t.Fatal("合法正则不应触碰用户的字面量偏好")
	}

	// 非法正则：自动字面量兜底，直接出结果、不报错
	m.input.SetValue("log.error(")
	triggerSearch(m)
	if m.searchErr != nil {
		t.Fatalf("非法正则应自动兜底而非报错: %v", m.searchErr)
	}
	if len(m.results) != 1 || m.results[0].Text != "log.error( here" {
		t.Fatalf("字面量兜底应命中该行: %+v", m.results)
	}
	if !strings.Contains(m.notice, "字面量") {
		t.Fatalf("状态栏应提示已按字面量搜索: %q", m.notice)
	}
	if m.matchLiteral {
		t.Fatal("自动兜底不应改变用户的 Ctrl+F 偏好")
	}

	// Ctrl+F 手动切到字面量后，正则写法按字面搜索（粘性开关生效）
	m.input.SetValue(`log\.(error|warn)`)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m.drain(cmd)
	if !m.matchLiteral || len(m.results) != 0 {
		t.Fatalf("字面量模式下正则写法应无命中: matchLiteral=%v results=%v", m.matchLiteral, m.results)
	}
	if !strings.Contains(m.statusView(), "字面") {
		t.Fatalf("状态栏应显示字面徽章:\n%s", m.statusView())
	}
}
