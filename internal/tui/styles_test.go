package tui

import (
	"image/color"
	"regexp"
	"testing"

	"charm.land/lipgloss/v2"
)

// sameColor 经 RGBA 分量比较两个 color.Color，规避具体底层类型差异。
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func resetTheme(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { ApplyTheme("", "", "", "") })
}

// TestThemePresetsValidHex 每套 preset 的每个槽位都必须是合法 hex，防止手抄色值笔误。
func TestThemePresetsValidHex(t *testing.T) {
	hexRe := regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	for name, p := range themePresets {
		for field, v := range map[string]string{
			"accent": p.accent, "match": p.match, "ok": p.ok, "dim": p.dim,
			"err": p.err, "white": p.white, "borderIdle": p.borderIdle,
			"statusFg": p.statusFg, "statusBg": p.statusBg, "badgeContentFg": p.badgeContentFg,
		} {
			if !hexRe.MatchString(v) {
				t.Errorf("preset %s 字段 %s 非法 hex: %q", name, field, v)
			}
		}
	}
}

// TestApplyThemeExplicitOverridesPreset 显式 hex 覆盖 preset 对应槽位。
func TestApplyThemeExplicitOverridesPreset(t *testing.T) {
	resetTheme(t)
	ApplyTheme("dracula", "#123456", "", "")
	if !sameColor(styleAppTitle.GetBackground(), lipgloss.Color("#123456")) {
		t.Errorf("显式 accent 应覆盖 preset: %v", styleAppTitle.GetBackground())
	}
	// accent 是槽位级覆盖：所有 accent 派生样式一起换色（与旧版 ApplyTheme 行为一致）
	if !sameColor(styleChipCursor.GetBackground(), lipgloss.Color("#123456")) {
		t.Error("chip 光标底色应同槽位换为显式 accent")
	}
	// 未覆盖的槽位保持 dracula preset 值
	if !sameColor(styleStatus.GetBackground(), lipgloss.Color("#282A36")) {
		t.Error("状态栏背景应保持 dracula preset 值")
	}
}

// TestApplyThemeUnknownPresetFallsBack 未知 preset 回退 default。
func TestApplyThemeUnknownPresetFallsBack(t *testing.T) {
	resetTheme(t)
	ApplyTheme("dracula", "", "", "")
	if !sameColor(styleStatus.GetBackground(), lipgloss.Color("#282A36")) {
		t.Fatal("前置失败：应先处于 dracula 主题")
	}
	ApplyTheme("nope", "", "", "")
	if !sameColor(styleStatus.GetBackground(), lipgloss.Color("#1B1B28")) {
		t.Error("未知 preset 应回退 default 的状态栏背景色")
	}
}

// TestChipAndPickerStylesFollowTheme 收编验证：chips 与选择器状态色随 preset 变化
// （修复前它们游离在 initStyles 之外，ApplyTheme 换色不生效）。
func TestChipAndPickerStylesFollowTheme(t *testing.T) {
	resetTheme(t)
	ApplyTheme("dracula", "", "", "")
	if !sameColor(styleChipCursor.GetBackground(), lipgloss.Color("#BD93F9")) {
		t.Error("chip 光标底色应随主题变化")
	}
	if !sameColor(styleChipActive.GetForeground(), lipgloss.Color("#8BE9FD")) {
		t.Error("chip 激活前景应随主题变化")
	}
	if !sameColor(styleStatusOK.GetForeground(), lipgloss.Color("#50FA7B")) {
		t.Error("选择器 OK 状态色应随主题变化")
	}
	if !sameColor(selRowStyle(10).GetForeground(), lipgloss.Color("#F8F8F2")) {
		t.Error("选中行前景应随主题变化")
	}
}
