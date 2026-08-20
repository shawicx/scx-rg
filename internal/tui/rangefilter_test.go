package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
)

func TestParseLineTime(t *testing.T) {
	now := time.Now()
	nginxZone := time.FixedZone("", 8*3600)
	cases := []struct {
		name string
		s    string
		want time.Time
		ok   bool
	}{
		{
			name: "docker/kubectl 快照 RFC3339 纳秒",
			s:    now.UTC().Format(time.RFC3339Nano) + " INFO started",
			want: now,
			ok:   true,
		},
		{
			name: "空格分隔秒级",
			s:    "2026-08-20 10:00:00 ERROR boom",
			want: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
			ok:   true,
		},
		{
			name: "nginx 括号时间",
			s:    "[21/Aug/2026:10:00:00 +0800] GET / HTTP/1.1",
			want: time.Date(2026, 8, 21, 10, 0, 0, 0, nginxZone),
			ok:   true,
		},
		{
			name: "syslog 补当年",
			s:    "Aug 20 10:00:05 host sshd[1]: hi",
			want: time.Date(time.Now().Year(), time.August, 20, 10, 0, 5, 0, time.UTC),
			ok:   true,
		},
		{name: "无时间戳", s: "plain error line", ok: false},
		{name: "空串", s: "", ok: false},
		{name: "行号文本", s: "2026 ERROR one", ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts, ok := parseLineTime(c.s)
			if ok != c.ok {
				t.Fatalf("ok = %v, 期望 %v（解析得 %v）", ok, c.ok, ts)
			}
			if !ok {
				return
			}
			d := ts.Sub(c.want)
			if d < 0 {
				d = -d
			}
			if d > 2*time.Minute {
				t.Fatalf("解析时间 %v 偏离期望 %v", ts, c.want)
			}
		})
	}
}

func TestRangeBarFilterByTime(t *testing.T) {
	recent := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	old := time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339Nano)
	m := newContentModel(t, map[string]string{
		"app.log": old + " ERROR old\n" + recent + " ERROR new\nERROR plain\n",
	})
	m.input.SetValue("ERROR")
	triggerSearch(m)
	if len(m.results) != 3 {
		t.Fatalf("前置: 应命中 3 条, 得到 %d", len(m.results))
	}

	// Ctrl+T 打开筛选栏，光标落在当前生效的「全部」
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if !m.rangeBar {
		t.Fatal("Ctrl+T 应打开筛选栏")
	}
	view := m.View()
	if !strings.Contains(view, "时间") || !strings.Contains(view, "条数") {
		t.Fatalf("筛选栏应显示时间/条数两段:\n%s", view)
	}

	// →→→ 移到「15分钟」，即时生效：旧时间戳行被滤掉，无时间戳行保留
	for i := 0; i < 3; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if m.filterDur != 15*time.Minute {
		t.Fatalf("filterDur = %v, 期望 15m", m.filterDur)
	}
	if len(m.results) != 2 {
		t.Fatalf("15 分钟筛选后应剩 2 条, 得到 %d", len(m.results))
	}
	if !strings.Contains(m.results[0].Text, "ERROR new") || !strings.Contains(m.results[1].Text, "ERROR plain") {
		t.Fatalf("筛选结果错误: %+v", m.results)
	}

	// Enter 关闭筛选栏：筛选保留，状态栏可见
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.rangeBar {
		t.Fatal("Enter 应关闭筛选栏")
	}
	view = m.View()
	if !strings.Contains(view, "15分钟") {
		t.Fatalf("状态栏应显示生效的时间筛选:\n%s", view)
	}
}

func TestRangeBarCapKeepsLatest(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 120; i++ {
		fmt.Fprintf(&b, "L%d ERROR\n", i)
	}
	m := newContentModel(t, map[string]string{"app.log": b.String()})
	m.input.SetValue("ERROR")
	triggerSearch(m)
	if len(m.results) != 120 {
		t.Fatalf("前置: 应命中 120 条, 得到 %d", len(m.results))
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})  // 切到「条数」段
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // 全部 → 100条
	if m.filterCap != 100 {
		t.Fatalf("filterCap = %d, 期望 100", m.filterCap)
	}
	if len(m.results) != 100 {
		t.Fatalf("封顶后应剩最新 100 条, 得到 %d", len(m.results))
	}
	if !strings.Contains(m.results[0].Text, "L21 ") || !strings.Contains(m.results[99].Text, "L120") {
		t.Fatalf("应保留最新的 100 条（L21..L120）: 首=%q 末=%q",
			m.results[0].Text, m.results[99].Text)
	}
}

func TestRangeFilterAppliesDuringStream(t *testing.T) {
	recent := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	old := time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339Nano)
	m := newContentModel(t, map[string]string{
		"app.log": old + " ERROR old\n" + recent + " ERROR new\n",
	})
	m.input.SetValue("ERROR")
	triggerSearch(m)

	// 打开筛选栏并选「15分钟」
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	for i := 0; i < 3; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if len(m.results) != 1 {
		t.Fatalf("前置: 筛选后应剩 1 条, 得到 %d", len(m.results))
	}

	// 发起新查询：流式摄取阶段就应过滤，raw 保留全量
	m.input.SetValue("ERROR")
	m.version++
	m.drain(m.runSearch())
	if len(m.raw) != 2 {
		t.Fatalf("raw 应保留全部命中 2 条, 得到 %d", len(m.raw))
	}
	if len(m.results) != 1 || !strings.Contains(m.results[0].Text, "ERROR new") {
		t.Fatalf("流式摄取应按时间过滤: %+v", m.results)
	}
}

func TestRangeBarCtrlCStillQuits(t *testing.T) {
	m := newContentModel(t, map[string]string{"a.txt": "needle\n"})
	m.input.SetValue("needle")
	triggerSearch(m)
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("筛选栏聚焦时 Ctrl+C 应触发退出 cmd")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("Ctrl+C 应返回 tea.Quit, 得到 %T", msg)
		}
	}
}

func TestRangeFilterInertWithoutTimestamps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "z.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, ImgProto: preview.ProtocolNone, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.input.SetValue("z")
	triggerSearch(m)
	if len(m.results) != 1 {
		t.Fatalf("前置: 文件模式应命中 1 条, 得到 %d", len(m.results))
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // 「1分钟」
	if m.filterDur != time.Minute {
		t.Fatalf("filterDur = %v, 期望 1m", m.filterDur)
	}
	if len(m.results) != 1 {
		t.Fatalf("无时间戳时筛选应失效（保留结果）, 得到 %d", len(m.results))
	}
	if m.tsOK {
		t.Fatal("文件名不应被检测出时间戳")
	}
	view := m.View()
	if !strings.Contains(view, "未检测到时间戳") {
		t.Fatalf("时间段应提示未检测到时间戳:\n%s", view)
	}
}
