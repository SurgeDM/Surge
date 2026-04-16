package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

// renderDetailsBox returns the file details pane as a btop box.
func (m *RootModel) renderDetailsBox(width, height int, selected *DownloadModel) string {
	contentWidth := width - 4
	contentHeight := height - 2

	if contentWidth < 0 {
		contentWidth = 0
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	var innerContent string
	if selected != nil {
		// renderFocusedDetails provides the formatted metrics block
		innerContent = renderFocusedDetails(selected, contentWidth, m.spinner.View())
	} else {
		// Default Placeholder
		innerContent = lipgloss.Place(contentWidth, contentHeight, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(colors.NeonCyan).Render("No Download Selected"))
	}

	return renderBtopBox("", PaneTitleStyle.Render(" File Details "), innerContent, width, height, colors.Gray)
}
