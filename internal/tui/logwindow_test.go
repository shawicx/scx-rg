package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

// 日志模式（PickLine）命中数超过窗口时必须保留「最新」的命中：
// 日志按时间顺序写入，旧命中滚出、新命中进来，否则永远只能看到最旧的一批。
func TestLogModeKeepsNewestMatches(t *testing.T) {
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= logWindow+5; i++ {
		fmt.Fprintf(&b, "L%d needle\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker.log"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:        dir,
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		PickLine:    true,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.onceMode = true
	m.input.SetValue("needle")
	triggerSearch(m)

	if len(m.raw) != logWindow {
		t.Fatalf("raw 应滑动保留最新 %d 条, 得到 %d", logWindow, len(m.raw))
	}
	if len(m.results) != logWindow {
		t.Fatalf("结果应重建为最新窗口 %d 条, 得到 %d", logWindow, len(m.results))
	}
	if !strings.Contains(m.results[0].Text, "L6 ") {
		t.Fatalf("最旧的 L1-L5 应滚出窗口, 首条=%q", m.results[0].Text)
	}
	if last := m.results[logWindow-1].Text; !strings.Contains(last, "L5005") {
		t.Fatalf("最新的 L5005 必须在结果中, 末条=%q", last)
	}
}

// 代码搜索（非日志）行为不变：仍是前 500 条封顶并杀掉 rg。
func TestCodeModeStillCapsFirstMatches(t *testing.T) {
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&b, "L%d needle\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:        dir,
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.onceMode = true
	m.input.SetValue("needle")
	triggerSearch(m)

	if len(m.results) != search.MaxResults {
		t.Fatalf("代码模式应仍封顶 %d 条, 得到 %d", search.MaxResults, len(m.results))
	}
	if !strings.Contains(m.results[0].Text, "L1 ") || !strings.Contains(m.results[search.MaxResults-1].Text, "L500") {
		t.Fatalf("代码模式应保留前 500 条: 首=%q 末=%q",
			m.results[0].Text, m.results[search.MaxResults-1].Text)
	}
}

// 日志模式下时间筛选与滑动窗口叠加：窗口内再按时间/条数过滤。
func TestLogModeWindowRespectsTimeFilter(t *testing.T) {
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	now := time.Now()
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= logWindow+5; i++ {
		age := 10 * time.Minute // 全部在 10 分钟前（10 分钟窗内）
		if i <= 5 {
			age = time.Hour // 仅最旧 5 条在 1 小时前
		}
		fmt.Fprintf(&b, "%s L%d needle\n", now.Add(-age).UTC().Format(time.RFC3339Nano), i)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker.log"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Root:        dir,
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		PickLine:    true,
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.onceMode = true
	m.now = func() time.Time { return now }
	m.input.SetValue("needle")
	triggerSearch(m)

	if len(m.results) != logWindow {
		t.Fatalf("前置: 窗口 %d 条, 得到 %d", logWindow, len(m.results))
	}
	if !strings.Contains(m.results[0].Text, "L6 ") {
		t.Fatalf("窗口首条应为 L6: %q", m.results[0].Text)
	}
	// 激活 15 分钟时间筛选：1 小时前的旧行（已滚出）不影响，全部窗口行保留
	m.filterDur = 15 * time.Minute
	m.drain(m.refilter(true))
	if len(m.results) != logWindow {
		t.Fatalf("15 分钟筛选后应保留 %d 条, 得到 %d", logWindow, len(m.results))
	}
}
