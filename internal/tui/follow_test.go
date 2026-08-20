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

func newFollowModel(t *testing.T, content string) (*Model, string) {
	t.Helper()
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:        dir,
		Mode:        ModeContent,
		Debounce:    time.Millisecond,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		FollowFile:  logPath,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.onceMode = true // 测试用 drain 同步驱动，禁用周期 tick
	m.followSize = int64(len(content))
	return m, logPath
}

func TestFollowTickRefreshesOnGrowth(t *testing.T) {
	m, logPath := newFollowModel(t, "2026 ERROR one\nplain\n")
	m.input.SetValue("ERROR")
	triggerSearch(m)
	if len(m.results) != 1 {
		t.Fatalf("前置: 初始 1 条, 得到 %d", len(m.results))
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("2026 ERROR two\n")
	f.Close()

	_, cmd := m.Update(followTickMsg{})
	if cmd == nil {
		t.Fatal("文件增长后 followTick 应触发刷新")
	}
	m.drain(cmd)
	if len(m.results) != 2 {
		t.Fatalf("刷新后应看到新命中 2 条, 得到 %d", len(m.results))
	}
	if !m.following() {
		t.Fatal("应为跟随状态")
	}
}

func TestFollowRefreshKeepsSelection(t *testing.T) {
	m, logPath := newFollowModel(t, "2026 ERROR one\n2026 ERROR two\n")
	m.input.SetValue("ERROR")
	triggerSearch(m)
	if len(m.results) != 2 {
		t.Fatalf("前置: 初始 2 条, 得到 %d", len(m.results))
	}
	// 用户选中第 2 条
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.sel != 1 || m.results[m.sel].Line != 2 {
		t.Fatalf("前置失败: sel=%d %+v", m.sel, m.results[m.sel])
	}

	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("2026 ERROR three\n")
	f.Close()

	_, cmd := m.Update(followTickMsg{})
	m.drain(cmd)

	if len(m.results) != 3 {
		t.Fatalf("刷新后 3 条, 得到 %d", len(m.results))
	}
	if m.sel != 1 || m.results[m.sel].Line != 2 {
		t.Fatalf("跟随刷新应保持选中（app.log:2）, 得到 sel=%d line=%d", m.sel, m.results[m.sel].Line)
	}
}
