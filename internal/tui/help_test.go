package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// 帮助浮层：? 在输入为空时打开；非空时作为搜索字符；任意键关闭。
func TestHelpOverlay(t *testing.T) {
	m := New(Config{Root: t.TempDir(), RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())

	// 输入非空时 ? 是搜索字符，不开浮层
	m.input.SetValue("que")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"}); cmd != nil {
		m.drain(cmd)
	}
	if m.helpOverlay {
		t.Fatal("输入非空时 ? 不应打开帮助")
	}

	// 清空输入后 ? 打开
	m.input.SetValue("")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"}); cmd != nil {
		m.drain(cmd)
	}
	if !m.helpOverlay {
		t.Fatal("输入为空时 ? 应打开帮助")
	}
	if v := m.frame(); !strings.Contains(v, "键位帮助") || !strings.Contains(v, "Ctrl+Space") {
		t.Fatalf("帮助帧应含键位表:\n%s", v)
	}

	// 任意键关闭
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace}); cmd != nil {
		m.drain(cmd)
	}
	if m.helpOverlay {
		t.Fatal("任意键应关闭帮助")
	}

	// F1 任何时候可用
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF1}); cmd != nil {
		m.drain(cmd)
	}
	if !m.helpOverlay {
		t.Fatal("F1 应打开帮助")
	}
	// 帮助打开时 Ctrl+C 仍直接退出
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("帮助中 Ctrl+C 应产生 Quit cmd")
	}
}
