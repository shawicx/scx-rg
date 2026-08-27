package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/logs"
	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

var pickerTestSources = []logs.Source{
	{Target: logs.Target{Kind: "docker", Name: "web"}, Detail: "nginx:stable", Status: "Up 2 days"},
	{Target: logs.Target{Kind: "docker", Name: "api"}, Detail: "repo/api:2.4", Status: "Exited (0) 1 hour ago"},
	{Target: logs.Target{Kind: "docker", Name: "worker"}, Detail: "worker:latest", Status: "Up 3 hours"},
}

func newPickerModel(t *testing.T, cfg Config) *Model {
	t.Helper()
	m := New(cfg)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.onceMode = true // 测试用 drain 同步驱动
	return m
}

func TestPickerLoadFilterAndPick(t *testing.T) {
	var fetched []logs.Target
	cfg := Config{
		PickerKind:  "docker",
		SnapshotDir: t.TempDir(),
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: search.RgAvailable(),
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return pickerTestSources, nil
		},
		FetchLog: func(ctx context.Context, tgt logs.Target) (string, error) {
			fetched = append(fetched, tgt)
			f, err := os.CreateTemp("", "snap-*.log")
			if err != nil {
				return "", err
			}
			_, _ = f.WriteString("hello needle\nplain\n")
			_ = f.Close()
			return f.Name(), nil
		},
	}
	m := newPickerModel(t, cfg)
	m.drain(m.Init())
	if !m.picking {
		t.Fatal("应处于选择器阶段")
	}
	if len(m.pickerView) != 3 {
		t.Fatalf("应加载 3 个容器, 得到 %d", len(m.pickerView))
	}
	view := m.frame()
	if !strings.Contains(view, "nginx:stable") {
		t.Fatalf("列表应显示镜像名:\n%s", view)
	}

	// 输入即时模糊过滤（名称+镜像）
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyExtended, Text: "api"})
	if len(m.pickerView) != 1 || m.pickerView[0].Target.Name != "api" {
		t.Fatalf("过滤后应只剩 api: %+v", m.pickerView)
	}

	// Enter 抓取快照并切入检索
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.drain(cmd)
	if m.picking {
		t.Fatal("Enter 后应切入检索阶段")
	}
	if len(fetched) != 1 || fetched[0].Name != "api" {
		t.Fatalf("应抓取 api 的日志: %+v", fetched)
	}
	if m.root != cfg.SnapshotDir {
		t.Fatalf("检索根应切到快照目录: %q", m.root)
	}
	if m.input.Value() != "" {
		t.Fatalf("切入检索时应清空过滤词, 得到 %q", m.input.Value())
	}
	if m.cfg.Title != "docker:api" {
		t.Fatalf("标题应为 docker:api, 得到 %q", m.cfg.Title)
	}
	if !m.cfg.PickLine {
		t.Fatal("日志快照场景 PickLine 应为 true")
	}

	// 空查询即应看到全部日志行（rg 空模式放行），而不是空列表
	if search.RgAvailable() {
		if len(m.results) != 2 { // "hello needle" + "plain"
			t.Fatalf("空查询应显示全部 2 行日志, 得到 %d", len(m.results))
		}
		if !strings.Contains(m.results[0].Text, "hello needle") {
			t.Fatalf("首条结果应为日志首行, 得到 %q", m.results[0].Text)
		}
		if view := m.frame(); !strings.Contains(view, "plain") {
			t.Fatalf("视图应包含日志内容:\n%s", view)
		}
		// 清空输入后回到全量，而不是空列表
		m.input.SetValue("zzz_no_hit")
		triggerSearch(m)
		if len(m.results) != 0 {
			t.Fatalf("无命中关键词应清空列表, 得到 %d", len(m.results))
		}
		m.input.SetValue("")
		triggerSearch(m)
		if len(m.results) != 2 {
			t.Fatalf("清空关键词应回到全量 2 行, 得到 %d", len(m.results))
		}
	}

	if search.RgAvailable() {
		m.input.SetValue("needle")
		triggerSearch(m)
		if len(m.results) != 1 {
			t.Fatalf("快照检索应命中 1 条, 得到 %d", len(m.results))
		}
	}
}

func TestPickerFollowPick(t *testing.T) {
	if !search.RgAvailable() {
		t.Skip("rg 未安装")
	}
	cfg := Config{
		PickerKind:  "docker",
		SnapshotDir: t.TempDir(),
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: true,
		FollowPick:  true,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return pickerTestSources[:1], nil
		},
		FollowLog: func(ctx context.Context, tgt logs.Target, path string) error {
			return os.WriteFile(path, []byte("ERROR one\nplain\n"), 0o644)
		},
	}
	m := newPickerModel(t, cfg)
	m.drain(m.Init())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.drain(cmd)

	if m.picking {
		t.Fatal("应切入检索阶段")
	}
	if !m.following() {
		t.Fatal("选中后应进入跟随模式")
	}
	// 空查询即应看到跟随进程落盘的全部初始内容
	if len(m.results) != 2 { // "ERROR one" + "plain"
		t.Fatalf("空查询应显示全部 2 行初始日志, 得到 %d", len(m.results))
	}
	m.input.SetValue("ERROR")
	triggerSearch(m)
	if len(m.results) != 1 {
		t.Fatalf("跟随初始内容应可检索, 得到 %d", len(m.results))
	}
}

func TestPickerFetchErrorStaysAndRetry(t *testing.T) {
	fetchErr := errors.New("docker daemon down")
	calls := 0
	cfg := Config{
		PickerKind:  "kubectl",
		SnapshotDir: t.TempDir(),
		Mode:        ModeContent,
		ImgProto:    preview.ProtocolNone,
		RgAvailable: false,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return pickerTestSources[:1], nil
		},
		FetchLog: func(ctx context.Context, tgt logs.Target) (string, error) {
			calls++
			if fetchErr != nil {
				return "", fetchErr
			}
			f, _ := os.CreateTemp("", "snap-*.log")
			_ = f.Close()
			return f.Name(), nil
		},
	}
	m := newPickerModel(t, cfg)
	m.drain(m.Init())

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.drain(cmd)
	if !m.picking {
		t.Fatal("抓取失败应停留在选择器")
	}
	if m.pickLoading {
		t.Fatal("失败后 pickLoading 应复位")
	}
	if m.searchErr == nil {
		t.Fatal("失败应展示错误")
	}

	// 恢复后重试成功
	fetchErr = nil
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.drain(cmd)
	if m.picking || calls != 2 {
		t.Fatalf("重试应成功切入检索: picking=%v calls=%d", m.picking, calls)
	}
}

func TestPickerEmptyListEnterNoop(t *testing.T) {
	cfg := Config{
		PickerKind:  "docker",
		SnapshotDir: t.TempDir(),
		ImgProto:    preview.ProtocolNone,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return nil, nil
		},
	}
	m := newPickerModel(t, cfg)
	m.drain(m.Init())
	if len(m.pickerView) != 0 {
		t.Fatal("前置: 空列表")
	}
	if !strings.Contains(m.frame(), "没有") {
		t.Fatalf("空列表应有提示:\n%s", m.frame())
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("空列表 Enter 应为空操作")
	}
	if !m.picking {
		t.Fatal("应仍处于选择器")
	}
}

func TestPickerCtrlRRefreshes(t *testing.T) {
	srcs := pickerTestSources[:1]
	cfg := Config{
		PickerKind:  "docker",
		SnapshotDir: t.TempDir(),
		ImgProto:    preview.ProtocolNone,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return srcs, nil
		},
	}
	m := newPickerModel(t, cfg)
	m.drain(m.Init())
	if len(m.pickerView) != 1 {
		t.Fatalf("前置: 1 个容器, 得到 %d", len(m.pickerView))
	}
	srcs = pickerTestSources
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+R 应触发刷新")
	}
	m.drain(cmd)
	if len(m.pickerView) != 3 {
		t.Fatalf("刷新后应 3 个容器, 得到 %d", len(m.pickerView))
	}
}
