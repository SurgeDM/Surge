package tui

import (
	"sync"

	"github.com/SurgeDM/Surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

// === Layout Styles ===
var (
	AppStyle          lipgloss.Style
	PaneStyle         lipgloss.Style
	ActivePaneStyle   lipgloss.Style
	LogoStyle         lipgloss.Style
	GraphStyle        lipgloss.Style
	ListStyle         lipgloss.Style
	DetailStyle       lipgloss.Style
	TitleStyle        lipgloss.Style
	PaneTitleStyle    lipgloss.Style
	TabStyle          lipgloss.Style
	ActiveTabStyle    lipgloss.Style
	StatsLabelStyle   lipgloss.Style
	StatsValueStyle   lipgloss.Style
	LogStyleStarted   lipgloss.Style
	LogStyleComplete  lipgloss.Style
	LogStyleError     lipgloss.Style
	LogStylePaused    lipgloss.Style
	WindowStyle       lipgloss.Style
	BoxStyle          lipgloss.Style
	ModalPaddingStyle lipgloss.Style
	LayoutGapStyle    lipgloss.Style
	EmptyMessageStyle lipgloss.Style
)

var styleOnce sync.Once

func InitializeStyles() {
	styleOnce.Do(func() {
		colors.RegisterThemeChangeHook(rebuildStyles)
		rebuildStyles()
	})
}

func rebuildStyles() {
	WindowStyle = lipgloss.NewStyle().Margin(DefaultPaddingY, DefaultPaddingX)
	BoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	ModalPaddingStyle = lipgloss.NewStyle().Padding(PopupPaddingY, PopupPaddingX)
	LayoutGapStyle = lipgloss.NewStyle().MarginTop(1)

	AppStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Foreground(colors.White())

	PaneStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray()).
		Padding(DefaultPaddingY, DefaultPaddingX)

	ActivePaneStyle = PaneStyle.BorderForeground(colors.Pink())
	LogoStyle = lipgloss.NewStyle().Foreground(colors.Magenta()).Bold(true).MarginBottom(1)
	GraphStyle = PaneStyle.BorderForeground(colors.Cyan())
	ListStyle = ActivePaneStyle // Download list is the primary focused pane on startup
	DetailStyle = PaneStyle

	TitleStyle = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true).MarginBottom(1)
	PaneTitleStyle = lipgloss.NewStyle().Foreground(colors.LightGray()).Bold(true)
	TabStyle = lipgloss.NewStyle().Foreground(colors.LightGray()).Padding(DefaultPaddingY, DefaultPaddingX)

	ActiveTabStyle = lipgloss.NewStyle().
		Foreground(colors.Pink()).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colors.Pink()).
		Padding(DefaultPaddingY, DefaultPaddingX).
		Bold(true)

	StatsLabelStyle = lipgloss.NewStyle().Foreground(colors.Cyan()).Width(12)
	StatsValueStyle = lipgloss.NewStyle().Foreground(colors.Pink()).Bold(true)

	LogStyleStarted = lipgloss.NewStyle().Foreground(colors.StateDownloading())
	LogStyleComplete = lipgloss.NewStyle().Foreground(colors.StateDone())
	LogStyleError = lipgloss.NewStyle().Foreground(colors.StateError())
	LogStylePaused = lipgloss.NewStyle().Foreground(colors.StatePaused())
	EmptyMessageStyle = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(false).Italic(true)
}
