package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"scx-rg/internal/search"
)

// fakeBlame 返回固定两行文件的 porcelain。
func fakeBlame() string {
	ts := time.Now().Add(-26 * time.Hour).Unix()
	full := "1234567890123456789012345678901234567890"
	return strings.Join([]string{
		full + " 1 1 1",
		"author Alice",
		"author-time " + strconv.FormatInt(ts, 10),
		"summary s1",
		"\tline one",
		full + " 2 2 1",
		"author Alice",
		"author-time " + strconv.FormatInt(ts, 10),
		"summary s2",
		"\tline two",
	}, "\n")
}

func newBlameModel(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:      dir,
		ShowBlame: true,
		BlameFetch: func(context.Context, string, string) (string, error) {
			return fakeBlame(), nil
		},
	})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	return m
}

func TestBlameSummaryShownOnSelection(t *testing.T) {
	m := newBlameModel(t)
	m.results = []search.Result{{Path: "a.txt", Line: 2}}
	m.sel = 0
	m.drain(m.followSelection())
	if !strings.Contains(m.frame(), "1234567 Alice 1d") {
		t.Errorf("状态栏应显示 blame 摘要:\n%s", m.frame())
	}
}

// 缓存：第二次选中同行零拉取；mtime 变化失效重拉。
func TestBlameCacheHitAndInvalidate(t *testing.T) {
	m := newBlameModel(t)
	m.blameCache = newBlameCache() // Init 的空查询已预热缓存，重置后从零计数
	m.blameText = ""
	fetches := 0
	m.cfg.BlameFetch = func(context.Context, string, string) (string, error) {
		fetches++
		return fakeBlame(), nil
	}
	m.results = []search.Result{{Path: "a.txt", Line: 1}}
	m.sel = 0
	m.drain(m.followSelection())
	if fetches != 1 {
		t.Fatalf("首次应拉取: %d", fetches)
	}
	m.drain(m.followSelection()) // 同行免拉
	if fetches != 1 {
		t.Errorf("缓存命中不应再拉取: %d", fetches)
	}
	// 换行：命中同一份文件缓存
	m.results = []search.Result{{Path: "a.txt", Line: 2}}
	m.drain(m.followSelection())
	if fetches != 1 {
		t.Errorf("同文件换行应命中缓存: %d", fetches)
	}
	if !strings.Contains(m.frame(), "1234567 Alice 1d") {
		t.Error("换行后摘要应更新")
	}
	// mtime 变化 → 失效重拉
	future := time.Now().Add(time.Hour).Unix()
	origStat := statFile
	statFile = func(string) (int64, error) { return future, nil }
	t.Cleanup(func() { statFile = origStat })
	m.results = []search.Result{{Path: "a.txt", Line: 1}}
	m.drain(m.followSelection())
	if fetches != 2 {
		t.Errorf("mtime 变化应重拉: %d", fetches)
	}
}

// Ctrl+B 关闭/开启；拉取失败静默。
func TestBlameToggleAndSilentFailure(t *testing.T) {
	m := newBlameModel(t)
	m.blameCache = newBlameCache()
	m.blameText = ""
	m.cfg.BlameFetch = func(context.Context, string, string) (string, error) {
		return "", context.DeadlineExceeded
	}
	m.results = []search.Result{{Path: "a.txt", Line: 1}}
	m.drain(m.followSelection())
	if m.blameText != "" {
		t.Error("拉取失败应静默")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}); cmd != nil {
		m.drain(cmd)
	}
	if m.blameOn {
		t.Error("Ctrl+B 应关闭 blame")
	}
}

// blameCache LRU 容量淘汰。
func TestBlameCacheLRUEviction(t *testing.T) {
	c := newBlameCache()
	lines := map[int]string{1: "x"}
	for i := 0; i < blameCacheCap+5; i++ {
		c.put("f"+string(rune('a'+i%26))+strconv.Itoa(i), 1, lines)
	}
	if len(c.entries) > blameCacheCap {
		t.Errorf("容量应封顶 %d: %d", blameCacheCap, len(c.entries))
	}
	if len(c.order) != len(c.entries) {
		t.Error("order 与 entries 应一致")
	}
}
