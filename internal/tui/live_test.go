package tui

import (
	"context"
	"fmt"
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

// TestLiveDirectKeepsPickerKind 直达实时（LiveTargets 非空）必须保留
// pickerKind：头部徽标与实时状态栏都按它渲染 kind，缺失时显示「 实时  」
// 空缺；Ctrl+R 重选的列源类型同样依赖（见 TestLiveDirectReenterStaysLive）。
func TestLiveDirectKeepsPickerKind(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}},
		fakeStream(map[string][]string{"web": {"l"}}))
	m.drain(m.Init())
	if m.pickerKind != "docker" {
		t.Fatalf("直达实时应保留 pickerKind, got %q", m.pickerKind)
	}
	if f := m.frame(); !strings.Contains(f, "实时 docker") {
		t.Fatalf("帧内徽标应含 kind（实时 docker）:\n%s", f)
	}
}

// TestLiveDirectReenterStaysLive 直达实时会话 Ctrl+R 重进选择器后必须保持
// 实时语义：pickerKind 决定重选列源类型（k8s 会话不得误列 docker 容器），
// LivePick 决定 Enter 分流——回实时多面板而非退化成 --snapshot 快照检索。
func TestLiveDirectReenterStaysLive(t *testing.T) {
	m := New(Config{
		PickerKind:  "docker",
		LivePick:    true,
		LiveDir:     t.TempDir(),
		LiveTargets: []logs.Target{{Kind: "docker", Name: "web"}},
		Mode:        ModeContent,
		StreamLog:   fakeStream(map[string][]string{"web": {"l"}}),
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			if kind != "docker" {
				t.Errorf("重选列源应以会话原 kind（docker）, got %q", kind)
			}
			return []logs.Source{{Target: logs.Target{Kind: "docker", Name: "web"}}}, nil
		},
	})
	m.onceMode = true // drain 同步驱动
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	if !m.liveMode {
		t.Fatal("LiveTargets 非空应直达实时")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.drain(cmd)
	if !m.picking {
		t.Fatal("Ctrl+R 应回到选择器")
	}
	if len(m.pickerView) != 1 {
		t.Fatalf("重选应加载源列表, got %d", len(m.pickerView))
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.drain(cmd)
	if !m.liveMode {
		t.Fatal("LivePick 会话 Enter 应回实时而非快照")
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
	// live-3 是唯一非对称布局（上 1 全宽 + 下 2 左右），必须有视觉基线
	goldenFrame(t, "live-3", liveFrame(t, map[string][]string{
		"api": {"api-line"}, "cache": {"cache-line"}, "web": {"web-line"},
	}))
	goldenFrame(t, "live-4", liveFrame(t, map[string][]string{
		"a1": {"line-a"}, "b2": {"line-b"}, "c3": {"line-c"}, "d4": {"line-d"},
	}))
}

// longStream 写 n 行后长驻到 ctx 取消（模拟运行中的容器）：
// drain 靠 liveDrainWait 限时收束读链，不走流自然结束路径。
func longStream(n int) func(context.Context, logs.Target, int, string, func(string)) error {
	return func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
		for i := 0; i < n; i++ {
			onLine(fmt.Sprintf("line-%02d", i))
		}
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestLiveScrollPausesFollow(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}},
		func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
			for i := 0; i < 60; i++ {
				onLine(fmt.Sprintf("line-%02d", i))
			}
			<-ctx.Done()
			return ctx.Err()
		})
	m.drain(m.Init())
	p := m.livePanels[0]
	p.vp.SetYOffset(0) // 直接翻到顶，模拟上翻
	p.follow = false
	m.scrollLive(-1) // 再往上滚一行
	if p.follow {
		t.Fatal("离开底部应暂停跟随")
	}
	bottomBefore := max(0, len(p.buf)-p.vp.Height())
	m.gotoLiveBottom()
	if !p.follow || p.vp.YOffset() != bottomBefore {
		t.Fatalf("G 应回底并恢复跟随: follow=%v y=%d want=%d", p.follow, p.vp.YOffset(), bottomBefore)
	}
}

func TestLiveFocusSwitch(t *testing.T) {
	m := newLiveModel(t,
		[]logs.Target{{Kind: "docker", Name: "a"}, {Kind: "docker", Name: "b"}},
		fakeStream(map[string][]string{"a": {"la"}, "b": {"lb"}}))
	m.drain(m.Init())
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.liveFocus != 1 {
		t.Fatalf("Tab 应切到 2 号面板, got %d", m.liveFocus)
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "3"})
	// 只有 2 个面板：3 应被忽略
	if m.liveFocus != 1 {
		t.Fatalf("越界数字键应忽略, got %d", m.liveFocus)
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "2"})
	if m.liveFocus != 1 {
		t.Fatalf("数字键 2 应直达 2 号面板, got %d", m.liveFocus)
	}
}

func TestLiveCopySearchCommand(t *testing.T) {
	var copied string
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}}, fakeStream(map[string][]string{"web": {"l"}}))
	m.writeClipboard = func(s string) error { copied = s; return nil }
	m.drain(m.Init())
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "y"})
	want := "scx-rg --follow " + m.livePanels[0].path
	if copied != want {
		t.Fatalf("y 应复制搜索命令: %q want %q", copied, want)
	}
	if !strings.Contains(m.notice, "已复制") {
		t.Fatalf("应有复制提示: %q", m.notice)
	}
}

func TestLiveHelpOverlay(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}}, fakeStream(map[string][]string{"web": {"l"}}))
	m.drain(m.Init())
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	if !m.helpOverlay {
		t.Fatal("F1 应打开帮助")
	}
	if f := m.frame(); !strings.Contains(f, "实时日志") {
		t.Fatalf("帮助应含实时分组:\n%s", f)
	}
}

// TestLiveResizeKeepsPausedOffset resize 重建视口须保留暂停跟随面板的偏移：
// viewport.New 出来的新视口 YOffset 从 0 开始，不保存旧偏移就会静默回顶
// （跟随面板贴底不受影响，只有上翻暂停态会丢位置）。
func TestLiveResizeKeepsPausedOffset(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}}, longStream(60))
	m.drain(m.Init())
	p := m.livePanels[0]
	p.vp.SetYOffset(10) // 模拟上翻到中部并暂停跟随
	p.follow = false
	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if p.follow {
		t.Fatal("resize 不应改变跟随标志")
	}
	if p.vp.YOffset() != 10 {
		t.Fatalf("resize 应保留暂停偏移 10, got %d", p.vp.YOffset())
	}
}

// TestLivePausedScrollSyncsNewLines 暂停跟随期间新行只入缓冲不重建视口
// （liveLinesMsg 仅在 follow 时 rebuild），滚动/回底前须先把视口内容与
// 缓冲对齐——否则 maxY 按新缓冲计、SetYOffset 却被旧内容 clamp，G 会
// 落在过期底部。
func TestLivePausedScrollSyncsNewLines(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}},
		func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
			for i := 0; i < 60; i++ {
				onLine(fmt.Sprintf("line-%02d", i))
			}
			return nil // 短流收束：缓冲定格，后续行由测试直接注入消息
		})
	m.drain(m.Init())
	p := m.livePanels[0]
	p.vp.SetYOffset(0)
	p.follow = false
	// 暂停期间新行到达：只入缓冲，视口内容冻结在暂停时刻
	_, _ = m.Update(liveLinesMsg{seq: m.liveSeq, panel: 0, lines: []string{"extra"}})
	if p.vp.YOffset() != 0 || len(p.buf) != 61 {
		t.Fatalf("暂停态新行应入缓冲不动视口: y=%d buf=%d", p.vp.YOffset(), len(p.buf))
	}
	m.gotoLiveBottom()
	want := max(0, len(p.buf)-p.vp.Height())
	if !p.follow || p.vp.YOffset() != want {
		t.Fatalf("G 应回到含新行的缓冲底部: follow=%v y=%d want=%d", p.follow, p.vp.YOffset(), want)
	}
}

// TestLivePausedGotoTopSyncsNewLines g/Home 到顶与 G 回底同理须先对齐内容：
// 偏移 0 并不免除 SetContentLines——暂停期间新行只入缓冲不重建视口（且
// 环形窗口可能已淘汰旧行），直接对冻结内容 SetYOffset(0) 显示的是过期
// 窗口顶，视口行数与 buf 脱节就是下次按键跳变的前兆。
func TestLivePausedGotoTopSyncsNewLines(t *testing.T) {
	m := newLiveModel(t, []logs.Target{{Kind: "docker", Name: "web"}},
		func(ctx context.Context, tgt logs.Target, tail int, path string, onLine func(string)) error {
			for i := 0; i < 60; i++ {
				onLine(fmt.Sprintf("line-%02d", i))
			}
			return nil // 短流收束：缓冲定格，后续行由测试直接注入消息
		})
	m.drain(m.Init())
	p := m.livePanels[0]
	p.vp.SetYOffset(0)
	p.follow = false
	// 暂停期间新行到达：只入缓冲，视口内容冻结在暂停时刻
	_, _ = m.Update(liveLinesMsg{seq: m.liveSeq, panel: 0, lines: []string{"extra"}})
	if p.vp.TotalLineCount() == len(p.buf) {
		t.Fatal("前置失效：暂停后视口内容应仍冻结（未同步新行）")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if p.follow || p.vp.YOffset() != 0 {
		t.Fatalf("Home 应到顶并暂停跟随: follow=%v y=%d", p.follow, p.vp.YOffset())
	}
	if p.vp.TotalLineCount() != len(p.buf) {
		t.Fatalf("到顶前视口应与缓冲对齐: 视口=%d buf=%d", p.vp.TotalLineCount(), len(p.buf))
	}
	if got := p.vp.GetContent(); !strings.HasPrefix(got, p.buf[0]) {
		t.Fatalf("视口顶应是缓冲首行（对齐后到顶）: got %q want 首行 %q", got, p.buf[0])
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: 'g'})
	if p.vp.YOffset() != 0 || p.follow {
		t.Fatalf("g 与 Home 应同效: follow=%v y=%d", p.follow, p.vp.YOffset())
	}
}
