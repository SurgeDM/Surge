package tui

import (
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
		innerContent = renderEmptyMessage(contentWidth, contentHeight, "No download selected")
	}

	return renderBtopBox("", PaneTitleStyle.Render(" File Details "), innerContent, width, height, colors.Gray)
}
