package tui

import (
	"github.com/surge-downloader/surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

// === Layout Styles ===
var (

	// The main box surrounding everything (optional, depending on terminal size)
	AppStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("0")). // Transparent/Default
			Foreground(colors.White)

	// Standard pane border
	PaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Gray).
			Padding(0, 1)

	// Focus style for the active pane
	ActivePaneStyle = PaneStyle.
			BorderForeground(colors.NeonPink)

	// === Specific Component Styles ===

	// 1. The "SURGE" Header
	LogoStyle = lipgloss.NewStyle().
			Foreground(colors.NeonPurple).
			Bold(true).
			MarginBottom(1)

	// 2. The Speed Graph (Top Right)
	GraphStyle = PaneStyle.
			BorderForeground(colors.NeonCyan)

	// 3. The Download List (Bottom Left)
	ListStyle = ActivePaneStyle // Usually focused by default

	// 4. The Detail View (Bottom Right)
	DetailStyle = PaneStyle

	// === Text Styles ===

	TitleStyle = lipgloss.NewStyle().
			Foreground(colors.NeonCyan).
			Bold(true).
			MarginBottom(1)

	// Helper for bold titles inside panes
	PaneTitleStyle = lipgloss.NewStyle().
			Foreground(colors.NeonCyan).
			Bold(true)

	TabStyle = lipgloss.NewStyle().
			Foreground(colors.LightGray).
			Padding(0, 1)

	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(colors.NeonPink).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colors.NeonPink).
			Padding(0, 1).
			Bold(true)

	StatsLabelStyle = lipgloss.NewStyle().
			Foreground(colors.NeonCyan).
			Width(12)

	StatsValueStyle = lipgloss.NewStyle().
			Foreground(colors.NeonPink).
			Bold(true)

	// Log Entry Styles
	LogStyleStarted = lipgloss.NewStyle().
			Foreground(colors.StateDownloading)

	LogStyleComplete = lipgloss.NewStyle().
				Foreground(colors.StateDone)

	LogStyleError = lipgloss.NewStyle().
			Foreground(colors.StateError)

	LogStylePaused = lipgloss.NewStyle().
			Foreground(colors.StatePaused)
)
