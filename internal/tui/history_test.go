package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestMain 全包隔离历史落盘路径：避免测试读写真实 ~/.local/state。
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "scx-rg-hist-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_STATE_HOME", tmp)
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func withHistoryEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	return filepath.Join(dir, "scx-rg", "history")
}

func TestRecordQueryDedupeAndPromote(t *testing.T) {
	withHistoryEnv(t) // 每测试独立目录：其他测试的记录不串扰
	m := New(Config{})
	m.recordQuery("app")
	m.recordQuery("app")  // 连续重复不记录
	m.recordQuery("user") // 新条目
	m.recordQuery("app")  // 旧条目上移
	if got := strings.Join(m.history, ","); got != "user,app" {
		t.Errorf("history = %q, want [user app]", got)
	}
	m.recordQuery("") // 空白忽略
	if len(m.history) != 2 {
		t.Errorf("空白不应记录: %v", m.history)
	}
}

func TestRecordQueryCap(t *testing.T) {
	withHistoryEnv(t)
	m := New(Config{HistorySize: 3})
	for _, q := range []string{"a", "b", "c", "d"} {
		m.recordQuery(q)
	}
	if got := strings.Join(m.history, ","); got != "b,c,d" {
		t.Errorf("超限应截断最旧: %q", got)
	}
}

func TestHistorySaveLoadRoundtrip(t *testing.T) {
	path := withHistoryEnv(t)
	saveHistory([]string{"旧查询", "新查询"}, 100)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("历史文件未落盘: %v", err)
	}
	got := loadHistory()
	if strings.Join(got, ",") != "旧查询,新查询" {
		t.Errorf("roundtrip = %v", got)
	}
	// 损坏行跳过、不阻断
	if err := os.WriteFile(path, []byte("\"ok\"\nnot-json\n\"ok2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadHistory(); strings.Join(got, ",") != "ok,ok2" {
		t.Errorf("坏行应跳过: %v", got)
	}
}

// Ctrl+G 打开浮层：最新在前；Enter 回填并执行；Del 删除单条。
func TestHistoryOverlayInteractions(t *testing.T) {
	withHistoryEnv(t)
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644)
	}
	m := New(Config{Root: dir, RgAvailable: false, HistorySize: 10})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.recordQuery("a")
	m.recordQuery("b") // 换成与文件名可模糊匹配的短查询
	m.drain(m.Init())

	// Ctrl+G 打开
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}); cmd != nil {
		m.drain(cmd)
	}
	if !m.historyOpen || !strings.Contains(m.frame(), "b") {
		t.Fatal("浮层应打开且最新在前")
	}
	// Del 删除最新一条（b）
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDelete}); cmd != nil {
		m.drain(cmd)
	}
	if len(m.history) != 1 {
		t.Errorf("Del 应删除当前条: %v", m.history)
	}
	// Enter 回填执行
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	if m.historyOpen || m.input.Value() != "a" {
		t.Errorf("Enter 应回填执行: open=%v value=%q", m.historyOpen, m.input.Value())
	}
	// 历史查询生效（a 过滤后仅 a.txt 模糊命中）
	if len(m.results) != 1 || m.results[0].Path != "a.txt" {
		t.Errorf("回填执行后应有 1 条 a.txt: %v", m.results)
	}
}

// Enter 选定时记录查询；shutdown 落盘。
func TestHistoryRecordedOnPick(t *testing.T) {
	path := withHistoryEnv(t)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644)
	m := New(Config{Root: dir, RgAvailable: false, HistorySize: 10})
	m.drain(m.Init())
	m.input.SetValue("a")
	m.version++
	m.drain(m.runSearch())
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		m.drain(cmd)
	}
	m.shutdown()
	got := loadHistory()
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("Enter 应记录查询并落盘: %v", got)
	}
	_ = path
}
