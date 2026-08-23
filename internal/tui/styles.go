package tui

import "charm.land/lipgloss/v2"

// borderASCII 宽度确定的 ASCII 边框。Unicode 制表符边框（╭─│╰╯）属于
// East Asian Ambiguous 宽度字符，在中文环境「歧义宽按 2 格」的终端里会
// 每格多占 1 列，导致整帧超宽、渲染器错位（界面鬼影），故全部改用 ASCII。
var borderASCII = lipgloss.Border{
	Top: "-", Bottom: "-", Left: "|", Right: "|",
	TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
}

// 主题三色：可被 ApplyTheme（config.toml [theme]）替换。
var (
	colorAccent = lipgloss.Color("#7D56F4") // 标题底色 / 激活边框 / 选中行
	colorCyan   = lipgloss.Color("#56C9F4") // 命中高亮 / prompt
	colorOK     = lipgloss.Color("#3DDC97") // 行标记 > ✓
)

var (
	colorDim = lipgloss.Color("#626262")
	colorErr = lipgloss.Color("#FF5C7A")

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
)

func init() { initStyles() }

// initStyles 是样式的唯一定义点；ApplyTheme 换色后重调即可全局生效。
func initStyles() {
	styleAppTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
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
		BorderForeground(lipgloss.Color("#3A3A4E"))

	styleDim = lipgloss.NewStyle().Foreground(colorDim)
	styleMatch = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleRowMarker = lipgloss.NewStyle().Bold(true).Foreground(colorOK)
	stylePanelTitle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	stylePlaceholder = lipgloss.NewStyle().Foreground(colorDim)
	styleErrText = lipgloss.NewStyle().Foreground(colorErr)
	styleSearching = lipgloss.NewStyle().Foreground(colorCyan)

	styleBadgeFiles = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(colorAccent).
		Padding(0, 1)

	styleBadgeContent = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#08111F")).
		Background(colorCyan).
		Padding(0, 1)

	styleStatus = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#C8C8D8")).
		Background(lipgloss.Color("#1B1B28"))

	stylePrompt = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
}

// ApplyTheme 应用配置文件的主题色（hex；空串保持默认）。须在 New 之前调用。
func ApplyTheme(accent, match, rowMarker string) {
	if accent != "" {
		colorAccent = lipgloss.Color(accent)
	}
	if match != "" {
		colorCyan = lipgloss.Color(match)
	}
	if rowMarker != "" {
		colorOK = lipgloss.Color(rowMarker)
	}
	initStyles()
}

func selRowStyle(w int) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(colorAccent).
		Width(w)
}
