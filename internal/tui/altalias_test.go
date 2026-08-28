package tui

// Alt 别名（堡垒机/浏览器 Web 终端规避 Ctrl 截获）的等效性测试：
// 每个 Alt+键 与对应 Ctrl+键 走同一动作；Alt 组合键不会漏进文本输入框。

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/logs"
)

// altKey 构造终端 ESC 前缀透传形态的 Alt 按键（Text 为空 → String() = "alt+x"）。
func altKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt}
}

func TestAltAliasRangeBarToggle(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())

	if _, cmd := m.Update(altKey('t')); cmd != nil {
		m.drain(cmd)
	}
	if !m.rangeBar {
		t.Fatal("Alt+T 应打开筛选栏（与 Ctrl+T 等效）")
	}
	// 筛选栏聚焦态下 Alt+T 关闭
	if _, cmd := m.Update(altKey('t')); cmd != nil {
		m.drain(cmd)
	}
	if m.rangeBar {
		t.Fatal("筛选栏内 Alt+T 应关闭（与 Ctrl+T/Esc 等效）")
	}
}

func TestAltAliasMatchToggle(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())
	before := m.matchExact

	if _, cmd := m.Update(altKey('f')); cmd != nil {
		m.drain(cmd)
	}
	if m.matchExact == before {
		t.Fatal("Alt+F 应切换匹配方式（与 Ctrl+F 等效）")
	}
}

func TestAltAliasMarkCopyAndNav(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())
	if len(m.results) == 0 {
		t.Fatal("前置失败：files 模式应有结果")
	}

	// Alt+M 标记 = Ctrl+Space（标记后选中自动下移，与 Ctrl+Space 行为一致）
	if _, cmd := m.Update(altKey('m')); cmd != nil {
		m.drain(cmd)
	}
	if m.markedCount() != 1 {
		t.Fatalf("Alt+M 应标记当前项, 标记数=%d", m.markedCount())
	}
	afterMark := m.sel
	// Alt+N / Alt+P 导航 = Ctrl+N / Ctrl+P
	if _, cmd := m.Update(altKey('n')); cmd != nil {
		m.drain(cmd)
	}
	if m.sel != afterMark+1 {
		t.Fatalf("Alt+N 应下移选中, sel=%d 期望 %d", m.sel, afterMark+1)
	}
	if _, cmd := m.Update(altKey('p')); cmd != nil {
		m.drain(cmd)
	}
	if m.sel != afterMark {
		t.Fatalf("Alt+P 应上移选中, sel=%d 期望 %d", m.sel, afterMark)
	}
	// Alt+Y 复制 = Ctrl+Y（注入剪贴板 fake）
	var copied string
	m.writeClipboard = func(s string) error { copied = s; return nil }
	if _, cmd := m.Update(altKey('y')); cmd != nil {
		m.drain(cmd)
	}
	if copied == "" {
		t.Fatal("Alt+Y 应复制选中路径")
	}
	if m.notice == "" {
		t.Fatal("Alt+Y 复制后应有状态提示")
	}
}

func TestAltAliasHistoryAndBlame(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())

	if _, cmd := m.Update(altKey('g')); cmd != nil {
		m.drain(cmd)
	}
	if !m.historyOpen {
		t.Fatal("Alt+G 应打开搜索历史（与 Ctrl+G 等效）")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
		m.drain(cmd)
	}
	if m.historyOpen {
		t.Fatal("前置失败：历史浮层应已关闭")
	}
	// 无结果时 Alt+B 仅翻转开关（requestBlame 有空结果 guard）
	if _, cmd := m.Update(altKey('b')); cmd != nil {
		m.drain(cmd)
	}
	if !m.blameOn {
		t.Fatal("Alt+B 应开启 blame（与 Ctrl+B 等效）")
	}
	if _, cmd := m.Update(altKey('b')); cmd != nil {
		m.drain(cmd)
	}
	if m.blameOn {
		t.Fatal("再次 Alt+B 应关闭 blame")
	}
}

func TestAltNotFedToTextinput(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())

	if _, cmd := m.Update(altKey('x')); cmd != nil {
		m.drain(cmd)
	}
	if m.input.Value() != "" {
		t.Fatalf("Alt 组合键不应进入搜索输入框, 得到 %q", m.input.Value())
	}
}

func TestAltAliasPickerRefresh(t *testing.T) {
	m := newPickerModel(t, Config{
		PickerKind:  "docker",
		SnapshotDir: t.TempDir(),
		Mode:        ModeContent,
		RgAvailable: false,
		ListSources: func(ctx context.Context, kind string) ([]logs.Source, error) {
			return pickerTestSources, nil
		},
	})
	m.drain(m.Init())

	// Alt+N/Alt+P 在选择器内导航
	if _, cmd := m.Update(altKey('n')); cmd != nil {
		m.drain(cmd)
	}
	if m.sel != 1 {
		t.Fatalf("选择器内 Alt+N 应下移, sel=%d", m.sel)
	}
	if _, cmd := m.Update(altKey('p')); cmd != nil {
		m.drain(cmd)
	}
	if m.sel != 0 {
		t.Fatalf("选择器内 Alt+P 应上移, sel=%d", m.sel)
	}
	// Alt+R 刷新列表 = Ctrl+R
	m.searchErr = errors.New("旧错误应被刷新清掉")
	if _, cmd := m.Update(altKey('r')); cmd != nil {
		m.drain(cmd)
	}
	if m.searchErr != nil {
		t.Fatal("Alt+R 应触发刷新并清空错误")
	}
	if len(m.pickerView) != len(pickerTestSources) {
		t.Fatalf("刷新后列表应完整, 得到 %d", len(m.pickerView))
	}
	// Alt 组合键不进过滤输入框
	if _, cmd := m.Update(altKey('x')); cmd != nil {
		m.drain(cmd)
	}
	if m.input.Value() != "" {
		t.Fatalf("选择器内 Alt 组合键不应进入过滤框, 得到 %q", m.input.Value())
	}
}

func TestPaletteEscapeHatchItems(t *testing.T) {
	m := goldenFilesModel(t)
	m.cfg.GitFiles = func(context.Context, string, bool) ([]string, error) {
		return nil, errors.New("not a repo")
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())

	titles := map[string]bool{}
	for _, it := range m.paletteItems() {
		titles[it.title] = true
	}
	for _, want := range []string{"复制选中", "翻页器打开选中", "标记/取消标记（多选）"} {
		if !titles[want] {
			t.Fatalf("命令面板应含无冲突逃生门条目 %q, 实际: %v", want, titles)
		}
	}
	// 编辑器条目仅在配置了 [editor] 时出现
	if titles["编辑器打开选中"] {
		t.Fatal("未配置编辑器时不应显示编辑器条目")
	}
	m.cfg.EditorCommand = "nvim"
	if found := func() bool {
		for _, it := range m.paletteItems() {
			if it.title == "编辑器打开选中" {
				return true
			}
		}
		return false
	}(); !found {
		t.Fatal("配置编辑器后应显示编辑器条目")
	}
}
