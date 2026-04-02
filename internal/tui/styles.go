package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

// === Layout Styles ===
var (
	AppStyle         lipgloss.Style
	PaneStyle        lipgloss.Style
	ActivePaneStyle  lipgloss.Style
	LogoStyle        lipgloss.Style
	GraphStyle       lipgloss.Style
	ListStyle        lipgloss.Style
	DetailStyle      lipgloss.Style
	TitleStyle       lipgloss.Style
	PaneTitleStyle   lipgloss.Style
	TabStyle         lipgloss.Style
	ActiveTabStyle   lipgloss.Style
	StatsLabelStyle  lipgloss.Style
	StatsValueStyle  lipgloss.Style
	LogStyleStarted  lipgloss.Style
	LogStyleComplete lipgloss.Style
	LogStyleError    lipgloss.Style
	LogStylePaused   lipgloss.Style
)

func init() {
	colors.RegisterThemeChangeHook(rebuildStyles)
	rebuildStyles()
}

func rebuildStyles() {
	AppStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Foreground(colors.White)

	PaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray).
		Padding(0, 1)

	ActivePaneStyle = PaneStyle.BorderForeground(colors.NeonPink)
	LogoStyle = lipgloss.NewStyle().Foreground(colors.NeonPurple).Bold(true).MarginBottom(1)
	GraphStyle = PaneStyle.BorderForeground(colors.NeonCyan)
	ListStyle = ActivePaneStyle // Download list is the primary focused pane on startup
	DetailStyle = PaneStyle

	TitleStyle = lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true).MarginBottom(1)
	PaneTitleStyle = lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true)
	TabStyle = lipgloss.NewStyle().Foreground(colors.LightGray).Padding(0, 1)

	ActiveTabStyle = lipgloss.NewStyle().
		Foreground(colors.NeonPink).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colors.NeonPink).
		Padding(0, 1).
		Bold(true)

	StatsLabelStyle = lipgloss.NewStyle().Foreground(colors.NeonCyan).Width(12)
	StatsValueStyle = lipgloss.NewStyle().Foreground(colors.NeonPink).Bold(true)

	LogStyleStarted = lipgloss.NewStyle().Foreground(colors.StateDownloading)
	LogStyleComplete = lipgloss.NewStyle().Foreground(colors.StateDone)
	LogStyleError = lipgloss.NewStyle().Foreground(colors.StateError)
	LogStylePaused = lipgloss.NewStyle().Foreground(colors.StatePaused)
}
