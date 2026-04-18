package components

import (
	"image/color"

	"github.com/SurgeDM/Surge/internal/tui/colors"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// ConfirmationModal renders a styled confirmation dialog box
type ConfirmationModal struct {
	Title       string
	Message     string
	Detail      string      // Optional additional detail line (e.g., filename, URL)
	Keys        help.KeyMap // Key bindings to show in help
	Help        help.Model  // Help model for rendering keys
	BorderColor color.Color // Border color for the box
	Width       int
	Height      int
}

// NoKeys satisfies help.KeyMap for informational modals with no interactive bindings.
type NoKeys struct{}

func (NoKeys) ShortHelp() []key.Binding  { return nil }
func (NoKeys) FullHelp() [][]key.Binding { return nil }

// View renders the confirmation modal content (without the box wrapper or help text)
func (m ConfirmationModal) view() string {
	detailStyle := lipgloss.NewStyle().
		Foreground(colors.Magenta()).
		Bold(true)

	// Build content - just message and detail (no help)
	content := m.Message

	if m.Detail != "" {
		content = lipgloss.JoinVertical(lipgloss.Center,
			content,
			"",
			detailStyle.Render(m.Detail),
		)
	}

	return content
}

// RenderWithBtopBox renders the modal using the btop-style box with title in border
// Help text is pushed to the last line of the modal
func (m ConfirmationModal) RenderWithBtopBox(
	renderBox func(leftTitle, rightTitle, content string, width, height int, borderColor color.Color) string,
	titleStyle lipgloss.Style,
) string {
	boxFrameX := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).GetHorizontalFrameSize()
	paddingX := lipgloss.NewStyle().Padding(0, 1).GetHorizontalFrameSize()
	innerWidth := m.Width - boxFrameX - paddingX

	boxFrameY := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).GetVerticalFrameSize()
	innerHeight := m.Height - boxFrameY

	// Get content without help
	mainContent := m.view()

	// Style and center help text
	helpStyle := lipgloss.NewStyle().
		Foreground(colors.Gray()).
		Width(innerWidth).
		Align(lipgloss.Center)
	helpText := helpStyle.Render(m.Help.View(m.Keys))

	// Calculate heights
	mainContentHeight := lipgloss.Height(mainContent)
	helpHeight := lipgloss.Height(helpText)

	// Space above content to vertically center the main content in remaining space
	spacingStyle := lipgloss.NewStyle().MarginBottom(1)
	remainingHeight := innerHeight - helpHeight - spacingStyle.GetVerticalFrameSize()
	topPadding := (remainingHeight - mainContentHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	// Center main content horizontally
	centeredMain := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(mainContent)

	// Build final content with help at bottom
	var lines []string
	for i := 0; i < topPadding; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, centeredMain)

	// Add padding to push help to bottom
	spacingNeeded := innerHeight - topPadding - mainContentHeight - helpHeight
	for i := 0; i < spacingNeeded; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, helpText)

	fullContent := lipgloss.JoinVertical(lipgloss.Left, lines...)

	// Title goes in the box border
	return renderBox(titleStyle.Render(" "+m.Title+" "), "", fullContent, m.Width, m.Height, m.BorderColor)
}

// Centered returns the modal centered in the given dimensions (for standalone use)
// Help text is pushed to the last line
func (m ConfirmationModal) Centered(width, height int) string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(m.BorderColor).
		Padding(1, 4)

	innerWidth := m.Width - boxStyle.GetHorizontalFrameSize()

	// Get content without help
	mainContent := m.view()

	// Style and center help text
	helpStyle := lipgloss.NewStyle().
		Foreground(colors.Gray()).
		Width(innerWidth).
		Align(lipgloss.Center)
	helpText := helpStyle.Render(m.Help.View(m.Keys))

	// Full content with spacing to push help down
	fullContent := lipgloss.JoinVertical(lipgloss.Center,
		mainContent,
		"",
		"",
		helpText,
	)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		boxStyle.Render(fullContent))
}
