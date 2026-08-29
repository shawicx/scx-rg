package tui

import (
	"context"
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
