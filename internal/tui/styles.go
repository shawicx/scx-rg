package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.Color("#7D56F4")
	colorCyan   = lipgloss.Color("#56C9F4")
	colorDim    = lipgloss.Color("#626262")
	colorText   = lipgloss.Color("#E6E6EF")
	colorErr    = lipgloss.Color("#FF5C7A")
	colorOK     = lipgloss.Color("#3DDC97")

	styleAppTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)

	styleInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	styleBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	styleBorderIdle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A4E"))

	styleDim         = lipgloss.NewStyle().Foreground(colorDim)
	styleMatch       = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleRowMarker   = lipgloss.NewStyle().Bold(true).Foreground(colorOK)
	stylePanelTitle  = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	stylePlaceholder = lipgloss.NewStyle().Foreground(colorDim)
	styleErrText     = lipgloss.NewStyle().Foreground(colorErr)
	styleSearching   = lipgloss.NewStyle().Foreground(colorCyan)

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
)

func selRowStyle(w int) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(colorAccent).
		Width(w)
}
