package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/preview"
)

// fakeKittyImage 模拟 kitty 协议渲染产物：信息行 + 删除前缀 + 图形序列 + 占位空行。
const fakeKittyImage = "shot.png / png / 4x4px\n" +
	preview.KittyDeleteImage +
	"\x1b_Gf=100,a=T,q=1,i=7,c=4,r=4,m=0;QUJDRA==\x1b\\\n\n\n"

// newImageTestModel files 模型 + fake 渲染：.png 走 kitty 图形，其余按代码。
func newImageTestModel(t *testing.T, files map[string]string) *Model {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{Root: dir, RgAvailable: false, ImgProto: preview.ProtocolKitty})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.renderFile = func(path string, cols, rows int, proto preview.Protocol, jump int, query string) (preview.Rendered, error) {
		if strings.HasSuffix(path, ".png") {
			return preview.Rendered{Kind: preview.KindImage, Content: fakeKittyImage}, nil
		}
		return preview.Rendered{Kind: preview.KindCode, Content: "package main\n"}, nil
	}
	m.drain(m.Init())
	return m
}

// stepTo 用上下键把选中项移到 idx。
func stepTo(t *testing.T, m *Model, idx int) {
	t.Helper()
	for range 2 * len(m.results) {
		if m.sel == idx {
			return
		}
		key := tea.KeyDown
		if m.sel > idx {
			key = tea.KeyUp
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: key})
		m.drain(cmd)
	}
	t.Fatalf("无法导航到第 %d 项（当前 %d/%d）", idx, m.sel, len(m.results))
}

func findResult(m *Model, suffix string) int {
	for i, r := range m.results {
		if strings.HasSuffix(r.Path, suffix) {
			return i
		}
	}
	return -1
}

// kitty 图片 → 代码切换：新内容应带删除序列前缀，清掉 overlay 旧图。
func TestKittyGraphicClearedOnSwitchToCode(t *testing.T) {
	m := newImageTestModel(t, map[string]string{
		"shot.png": "png-bytes\n",
		"main.go":  "package main\n",
	})
	stepTo(t, m, findResult(m, ".png"))
	if !m.imgActive {
		t.Fatalf("渲染 kitty 图后 imgActive 应为 true:\n%s", m.vp.View())
	}

	stepTo(t, m, findResult(m, ".go"))
	if m.imgActive {
		t.Error("切到代码后 imgActive 应为 false")
	}
	if !strings.Contains(m.vp.View(), preview.KittyDeleteImage) {
		t.Errorf("切到代码后应注入删除序列（防 overlay 残留）:\n%q", m.vp.View())
	}
}

// 预览被清空（新搜索/无结果）：删除序列借空态提示帧送出，且只发一帧。
func TestKittyGraphicClearedWhenPreviewEmptied(t *testing.T) {
	m := newImageTestModel(t, map[string]string{"shot.png": "png-bytes\n"})
	stepTo(t, m, findResult(m, ".png"))
	if !m.imgActive {
		t.Fatal("前置失败：imgActive 应为 true")
	}

	m.input.SetValue("zzz-no-such-file")
	m.drain(m.runSearch())
	if m.prevPath != "" {
		t.Fatalf("runSearch 后预览应清空，prevPath=%q", m.prevPath)
	}
	if v := m.frame(); !strings.Contains(v, preview.KittyDeleteImage) {
		t.Errorf("清空后的帧应送出删除序列:\n%q", v)
	}
	if m.imgActive {
		t.Error("序列送出后 imgActive 标志应就地消费")
	}
	if v := m.frame(); strings.Contains(v, preview.KittyDeleteImage) {
		t.Error("删除序列只应随首帧发送一次")
	}
}

// 图片预览禁用 PgUp/PgDn：图形不随文本滚动，滚动只会错位。
func TestImagePreviewDisablesScroll(t *testing.T) {
	m := newImageTestModel(t, map[string]string{"shot.png": "png-bytes\n"})
	m.prevKind = string(preview.KindImage)
	m.vp.SetContent(strings.Repeat("line\n", 100))
	m.vp.SetYOffset(0)

	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown}); cmd != nil {
		m.drain(cmd)
	}
	if m.vp.YOffset() != 0 {
		t.Errorf("图片预览不应滚动，YOffset=%d", m.vp.YOffset())
	}

	m.prevKind = string(preview.KindCode)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown}); cmd != nil {
		m.drain(cmd)
	}
	if m.vp.YOffset() == 0 {
		t.Error("代码预览应可正常滚动")
	}
}
