package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRapidNavigationFrameHeightInvariant 模拟真实渲染时序：快速按键时每个按键
// 后立即渲染一帧（不等异步 previewMsg 回来），瞬态帧行数必须恒等于终端高度——
// 帧行数瞬时超 height 会被 bubbletea 渲染器截顶，造成整帧错位鬼影。
func TestRapidNavigationFrameHeightInvariant(t *testing.T) {
	m := New(Config{Root: "../..", RgAvailable: false})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.drain(m.Init())
	bad := 0
	check := func(tag string) {
		n := strings.Count(m.View(), "\n") + 1
		if n != m.height {
			bad++
			if bad <= 5 {
				v := m.View()
				lines := strings.Split(v, "\n")
				t.Errorf("%s: 帧行数 %d != 高度 %d；超出行示例: %q / %q", tag, n, m.height,
					lines[min(40, len(lines)-1)], lines[len(lines)-1])
			}
		}
	}
	check("初始")
	for i := 0; i < 25; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown}) // 不 drain：渲染器看到的瞬态
		check("按下瞬间")
	}
	// 逐个补投递积压的 previewMsg（模拟异步陆续到达）
	m.drain(nil)
	check("回包后")
	for i := 0; i < 25; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyUp})
		check("按上瞬间")
	}
	m.drain(nil)
	check("回包后2")
	if bad == 0 {
		t.Log("瞬态帧行数全部正常")
	}
}
