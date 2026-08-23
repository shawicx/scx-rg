package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// borderASCII 宽度确定的 ASCII 边框。Unicode 制表符边框（╭─│╰╯）属于
// East Asian Ambiguous 宽度字符，在中文环境「歧义宽按 2 格」的终端里会
// 每格多占 1 列，导致整帧超宽、渲染器错位（界面鬼影），故全部改用 ASCII。
var borderASCII = lipgloss.Border{
	Top: "-", Bottom: "-", Left: "|", Right: "|",
	TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
}

// palette 主题调色板：TUI 外观的全套颜色槽位（hex）。
// preview 包的内容渲染色（行号槽/图片框）不参与主题。
type palette struct {
	accent         string // 标题底色 / 激活边框 / 选中行 / chip 光标底色
	match          string // 命中高亮 / prompt / chip 激活
	ok             string // 行标记 > ✓ / 状态正常
	dim            string // 次要文字
	err            string // 错误文字
	white          string // 标题与徽章前景
	borderIdle     string // 空闲边框
	statusFg       string // 状态栏前景
	statusBg       string // 状态栏背景
	badgeContentFg string // 内容/全文徽章前景（底色 = match）
}

// themePresets 命名主题（config.toml [theme] preset，及命令面板「切换主题」）。
// default 与历史内置色完全一致，保证既有 golden 基线零变化。
var themePresets = map[string]palette{
	"default": {
		accent: "#7D56F4", match: "#56C9F4", ok: "#3DDC97",
		dim: "#626262", err: "#FF5C7A", white: "#FFFFFF",
		borderIdle: "#3A3A4E", statusFg: "#C8C8D8", statusBg: "#1B1B28",
		badgeContentFg: "#08111F",
	},
	"dracula": {
		accent: "#BD93F9", match: "#8BE9FD", ok: "#50FA7B",
		dim: "#6272A4", err: "#FF5555", white: "#F8F8F2",
		borderIdle: "#44475A", statusFg: "#F8F8F2", statusBg: "#282A36",
		badgeContentFg: "#282A36",
	},
	"nord": {
		accent: "#81A1C1", match: "#88C0D0", ok: "#A3BE8C",
		dim: "#4C566A", err: "#BF616A", white: "#ECEFF4",
		borderIdle: "#3B4252", statusFg: "#D8DEE9", statusBg: "#2E3440",
		badgeContentFg: "#2E3440",
	},
	"catppuccin": {
		accent: "#CBA6F7", match: "#89B4FA", ok: "#A6E3A1",
		dim: "#6C7086", err: "#F38BA8", white: "#CDD6F4",
		borderIdle: "#313244", statusFg: "#CDD6F4", statusBg: "#1E1E2E",
		badgeContentFg: "#1E1E2E",
	},
}

// presetOrder 主题循环顺序（命令面板「切换主题」与会话内当前主题名）。
var presetOrder = []string{"default", "dracula", "nord", "catppuccin"}

// 主题三色（colorAccent/colorCyan/colorOK）与其余样式全部由 initStyles
// 统一构建；ApplyTheme 换色后重调即可全局生效。
var (
	colorAccent color.Color
	colorCyan   color.Color
	colorOK     color.Color
	colorDim    color.Color
	colorErr    color.Color
	colorWhite  color.Color

	styleAppTitle     lipgloss.Style
	styleInputBox     lipgloss.Style
	styleBorderActive lipgloss.Style
	styleBorderIdle   lipgloss.Style
	styleDim          lipgloss.Style
	styleMatch        lipgloss.Style
	styleRowMarker    lipgloss.Style
	stylePanelTitle   lipgloss.Style
	stylePlaceholder  lipgloss.Style
	styleErrText      lipgloss.Style
	styleSearching    lipgloss.Style
	styleBadgeFiles   lipgloss.Style
	styleBadgeContent lipgloss.Style
	styleStatus       lipgloss.Style
	stylePrompt       lipgloss.Style
	// 筛选栏 chips 三态（原定义在 rangefilter.go，收编进主题单一定义点）
	styleChipCursor lipgloss.Style
	styleChipActive lipgloss.Style
	styleChipIdle   lipgloss.Style
	// 选择器状态色（原定义在 picker.go）
	styleStatusOK  lipgloss.Style
	styleStatusBad lipgloss.Style
)

func init() { initStyles(themePresets["default"]) }

// initStyles 是样式的唯一定义点；ApplyTheme 换色后重调即可全局生效。
func initStyles(p palette) {
	colorAccent = lipgloss.Color(p.accent)
	colorCyan = lipgloss.Color(p.match)
	colorOK = lipgloss.Color(p.ok)
	colorDim = lipgloss.Color(p.dim)
	colorErr = lipgloss.Color(p.err)
	colorWhite = lipgloss.Color(p.white)

	styleAppTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWhite).
		Background(colorAccent).
		Padding(0, 1)

	styleInputBox = lipgloss.NewStyle().
		Border(borderASCII).
		BorderForeground(colorAccent).
		Padding(0, 1)

	styleBorderActive = lipgloss.NewStyle().
		Border(borderASCII).
		BorderForeground(colorAccent)

	styleBorderIdle = lipgloss.NewStyle().
		Border(borderASCII).
		BorderForeground(lipgloss.Color(p.borderIdle))

	styleDim = lipgloss.NewStyle().Foreground(colorDim)
	styleMatch = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleRowMarker = lipgloss.NewStyle().Bold(true).Foreground(colorOK)
	stylePanelTitle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	stylePlaceholder = lipgloss.NewStyle().Foreground(colorDim)
	styleErrText = lipgloss.NewStyle().Foreground(colorErr)
	styleSearching = lipgloss.NewStyle().Foreground(colorCyan)

	styleBadgeFiles = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWhite).
		Background(colorAccent).
		Padding(0, 1)

	styleBadgeContent = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(p.badgeContentFg)).
		Background(colorCyan).
		Padding(0, 1)

	styleStatus = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.statusFg)).
		Background(lipgloss.Color(p.statusBg))

	stylePrompt = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)

	styleChipCursor = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWhite).
		Background(colorAccent).
		Padding(0, 1)
	styleChipActive = lipgloss.NewStyle().Bold(true).Foreground(colorCyan).Padding(0, 1)
	styleChipIdle = styleDim.Padding(0, 1)

	styleStatusOK = lipgloss.NewStyle().Foreground(colorOK)
	styleStatusBad = lipgloss.NewStyle().Foreground(colorDim)
}

// ApplyTheme 应用命名主题与显式覆盖色（hex；空串保持对应槽位的 preset 值）。
// 优先级：显式色 > preset > default；未知 preset 回退 default。须在 New 之前调用。
func ApplyTheme(preset, accent, match, rowMarker string) {
	p, ok := themePresets[preset]
	if !ok {
		p = themePresets["default"]
	}
	if accent != "" {
		p.accent = accent
	}
	if match != "" {
		p.match = match
	}
	if rowMarker != "" {
		p.ok = rowMarker
	}
	initStyles(p)
}

func selRowStyle(w int) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWhite).
		Background(colorAccent).
		Width(w)
}
