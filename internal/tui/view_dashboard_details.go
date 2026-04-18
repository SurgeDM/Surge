package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

// renderDetailsBox returns the file details pane as a btop box.
func (m *RootModel) renderDetailsBox(width, height int, innerContent string) string {
	return renderBtopBox("", PaneTitleStyle.Render(" File Details "), innerContent, width, height, colors.Gray())
}
