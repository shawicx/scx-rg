package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestFrameFitsAmbiguousWideTerminals 防界面鬼影回归（2026-08-22 修复）：
// 中文环境终端常把 East Asian Ambiguous 字符（·…⋯⟳❯│╭× 等）按 2 格渲染，
// 而 lipgloss 按 1 格计宽——界面帧里只要出现这类字符，行宽就会在歧义宽终端
// 超出终端宽度，触发软换行，bubbletea 的 diff 渲染器随之行号错位（表现为
// 上下切换后输入框/标题重复残留）。
//
// 不变式（用不含歧义字符的预览内容验证；内容层折行由 preview 包测试覆盖）：
//  1. 帧行数恒等于终端高度；
//  2. 帧行宽在「歧义宽=2 格」口径下不超终端宽。
//
// 已知残留：用户文件内容若含歧义字符（如 README 中的 …），该行在歧义宽终端
// 仍会多占若干格（lipgloss 补边按歧义=1 计）——折行已按歧义宽保守计算，
// 溢出被限制在该行歧义字符数以内。
func TestFrameFitsAmbiguousWideTerminals(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"readme-zh.md": "# 项目说明\n\n" + strings.Repeat("全角字符宽度和折行行为验证内容。", 12) + "\nplain ascii line\n",
		"long.txt":     strings.Repeat("x", 300) + "\n",
		"app.go":       "package main\n\nfunc main() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	isAmbig := func(r rune) bool {
		switch {
		case r >= 0x2500 && r <= 0x25FF, // Box Drawing & Geometric Shapes
			r >= 0x2190 && r <= 0x21FF, // Arrows
			r >= 0x2200 && r <= 0x22FF: // Mathematical Operators（⋯ × 等）
			return true
		}
		return strings.ContainsRune("·…⋯⟳⏱⇥❯»›‹—✓✗⚠", r)
	}

	m := New(Config{Root: dir, RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())

	check := func(step string) {
		if step != "初始" {
			key := tea.KeyDown
			if step == "按上后" {
				key = tea.KeyUp
			}
			_, cmd := m.Update(tea.KeyMsg{Type: key})
			m.drain(cmd)
		}
		v := m.View()
		if n := strings.Count(v, "\n") + 1; n != m.height {
			t.Errorf("%s: 帧行数 %d 应等于终端高度 %d", step, n, m.height)
		}
		for i, l := range strings.Split(v, "\n") {
			inEsc := false
			ambigs := 0
			for _, r := range l {
				if r == 0x1b {
					inEsc = true
					continue
				}
				if inEsc {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
						inEsc = false
					}
					continue
				}
				if isAmbig(r) {
					ambigs++
				}
			}
			if w := lipgloss.Width(l) + ambigs; w > m.frameW() {
				t.Errorf("%s: 第 %d 行宽 %d 格 > frameW %d（应比终端少 1 列，见 frameW 注释）: %q", step, i+1, w, m.frameW(), l)
			}
		}
	}
	check("初始")
	check("按下后")
	check("按下后2")
	check("按上后")
}
