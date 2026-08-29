package tui

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/logs"
)

// fakeStream 逐行回调 script 后立即返回（模拟容器已退出的短流）。
func fakeStream(script map[string][]string) func(context.Context, logs.Target, int, string, func(string)) error {
	return func(ctx context.Context, t logs.Target, tail int, path string, onLine func(string)) error {
		for _, l := range script[t.Name] {
			onLine(l)
		}
		return nil
	}
}

// newLiveModel 直达实时模式的测试模型：ListSources 注入 fake——reenterPicker
// 后 drain 会消费 loadPicker 命令，为 nil 时会真跑 docker/kubectl。
func newLiveModel(t *testing.T, targets []logs.Target, stream func(context.Context, logs.Target, int, string, func(string)) error) *Model {
	t.Helper()
	m := New(Config{
		PickerKind: "docker", LiveDir: t.TempDir(), LiveTargets: targets,
		Mode: ModeContent, StreamLog: stream,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) { return nil, nil },
	})
	m.onceMode = true // drain 同步驱动
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func TestLiveRows(t *testing.T) {
	for n, want := range map[int][]int{
		1: {1}, 2: {1, 1}, 3: {1, 2}, 4: {2, 2},
	} {
		if got := liveRows(n); len(got) != len(want) {
			t.Fatalf("liveRows(%d)=%v want %v", n, got, want)
		}
	}
	if liveRows(0) != nil || liveRows(5) != nil {
		t.Fatal("面板数越界应返回 nil")
	}
}

func TestLiveAppendTrim(t *testing.T) {
	p := &livePanel{}
	lines := make([]string, logWindow+10)
	for i := range lines {
		lines[i] = "x"
	}
	p.appendLines(lines)
	if len(p.buf) != logWindow {
		t.Fatalf("环形窗口应封顶 %d, got %d", logWindow, len(p.buf))
	}
}

func TestLiveStartPipeline(t *testing.T) {
	m := newLiveModel(t,
		[]logs.Target{{Kind: "docker", Name: "web"}},
		fakeStream(map[string][]string{"web": {"hello-1", "hello-2"}}))
	m.drain(m.Init())
	if !m.liveMode {
		t.Fatal("LiveTargets 非空应进入实时模式")
	}
	if len(m.livePanels) != 1 {
		t.Fatalf("应建 1 个面板, got %d", len(m.livePanels))
	}
	joined := strings.Join(m.livePanels[0].buf, "|")
	if joined != "hello-1|hello-2" {
		t.Fatalf("行应按序入缓冲: %q", joined)
	}
	if !m.livePanels[0].exited {
		t.Fatal("流返回后面板应标记退出")
	}
}

func TestLiveDoneCarriesError(t *testing.T) {
	m := newLiveModel(t,
		[]logs.Target{{Kind: "docker", Name: "gone"}},
		func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
			onLine("first-line")
			return context.DeadlineExceeded
		})
	m.drain(m.Init())
	p := m.livePanels[0]
	if !p.exited || p.buf[len(p.buf)-1] != "⚠ context deadline exceeded" {
		t.Fatalf("错误应以 ⚠ 行入面板: exited=%v buf=%v", p.exited, p.buf)
	}
}

func TestLiveReenterPickerStopsStreams(t *testing.T) {
	// cancelled 用通道观察而非 bool：流 goroutine 的写与测试的读若无
	// 同步点构成数据竞态（-race 下必报）；close(stopped) 建立 happens-before。
	stopped := make(chan struct{})
	m := newLiveModel(t,
		[]logs.Target{{Kind: "docker", Name: "web"}},
		func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
			onLine("line")
			<-ctx.Done() // 长驻：模拟运行中的容器
			close(stopped)
			return ctx.Err()
		})
	m.drain(m.Init())
	if !m.liveMode {
		t.Fatal("应处于实时模式")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.drain(cmd)
	if m.liveMode || len(m.livePanels) != 0 {
		t.Fatal("Ctrl+R 应停流并清面板")
	}
	if !m.picking {
		t.Fatal("应回到选择器")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 应已取消")
	}
}

// liveFrame 构造确定性的实时模式整帧：面板按名字排序（golden 稳定），
// fakeStream 注入固定行后立即收束（无真实 docker 依赖）。
func liveFrame(t *testing.T, panels map[string][]string) string {
	t.Helper()
	var targets []logs.Target
	for name := range panels {
		targets = append(targets, logs.Target{Kind: "docker", Name: name})
	}
	// 固定顺序，golden 稳定
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	m := newLiveModel(t, targets, fakeStream(panels))
	m.drain(m.Init())
	// 状态栏会显示落盘目录：t.TempDir() 是机器相关的随机路径，golden 基线
	// 必须确定性（goldenFrame 的无随机路径约束），帧渲染前换成固定展示值。
	// MkdirAll 在 startLive 时已用真实临时目录执行过，此处只影响显示。
	m.cfg.LiveDir = "/var/tmp/scx-rg-live"
	return m.frame()
}

func TestLiveRenderOnePanel(t *testing.T) {
	f := liveFrame(t, map[string][]string{"web": {"line-1", "line-2"}})
	for _, want := range []string{"web", "line-1", "line-2"} {
		if !strings.Contains(f, want) {
			t.Fatalf("单面板帧应含 %q:\n%s", want, f)
		}
	}
}

func TestLiveRenderTwoPanels(t *testing.T) {
	f := liveFrame(t, map[string][]string{"api": {"api-log"}, "web": {"web-log"}})
	if !strings.Contains(f, "api") || !strings.Contains(f, "web") ||
		!strings.Contains(f, "api-log") || !strings.Contains(f, "web-log") {
		t.Fatalf("双面板帧应同时含两容器与日志:\n%s", f)
	}
}

func TestLiveGolden(t *testing.T) {
	goldenFrame(t, "live-1", liveFrame(t, map[string][]string{"web": {"2026-08-30T10:00:00Z hello", "2026-08-30T10:00:01Z world"}}))
	goldenFrame(t, "live-2", liveFrame(t, map[string][]string{"api": {"api-line"}, "web": {"web-line"}}))
	goldenFrame(t, "live-4", liveFrame(t, map[string][]string{
		"a1": {"line-a"}, "b2": {"line-b"}, "c3": {"line-c"}, "d4": {"line-d"},
	}))
}
