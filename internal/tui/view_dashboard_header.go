package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
)

// renderHeaderBox displays the Surge logo and the server connection status within a box.
func (m *RootModel) renderHeaderBox(width, height int) string {
	contentWidth := width - components.BorderFrameWidth
	contentHeight := height - components.BorderFrameHeight

	if contentWidth < 0 {
		contentWidth = 0
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	logoText := `   _______  ___________ ____ 
  / ___/ / / / ___/ __ '/ _ \
 (__  ) /_/ / /  / /_/ /  __/
/____/\__,_/_/   \__, /\___/ 
                /____/       `

	compactLogoText := `   _____
  / ___/
 (__  )
/____/`

	// Server info part
	greenDot := lipgloss.NewStyle().Foreground(colors.StateDownloading()).Render("\u25cf")
	host := m.ServerHost
	if host == "" {
		host = "127.0.0.1"
	}
	serverAddr := fmt.Sprintf("%s:%d", host, m.ServerPort)

	var statusLine string
	if contentWidth < 28 {
		// Just show the address when narrow
		statusLine = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true).Render(" " + serverAddr)
	} else if m.IsRemote {
		statusLine = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true).Render(" Connected to " + serverAddr)
	} else {
		statusLine = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true).Render(" Serving at " + serverAddr)
	}

	serverPortContent := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Render(greenDot + statusLine)

	var innerContent string
	// If the height is too short for both logo and server text, just return server text centered vertically
	if contentHeight < 4 {
		innerContent = lipgloss.Place(contentWidth, contentHeight, lipgloss.Center, lipgloss.Center, serverPortContent)
	} else if width < MinLogoWidth {
		// Show compact logo for medium-short headers or narrow terminals
		logoContent := ApplyGradient(compactLogoText, colors.Pink(), colors.Magenta())
		logoBoxHeight := contentHeight - components.SingleLineHeight // 1 line for the server text at the bottom
		logoBox := lipgloss.Place(contentWidth, logoBoxHeight, lipgloss.Center, lipgloss.Center, logoContent)
		innerContent = lipgloss.JoinVertical(lipgloss.Center, logoBox, serverPortContent)
	} else {
		var logoContent string
		if m.logoCache != "" {
			logoContent = m.logoCache
		} else {
			gradientLogo := ApplyGradient(logoText, colors.Pink(), colors.Magenta())
			m.logoCache = lipgloss.NewStyle().Render(gradientLogo)
			logoContent = m.logoCache
		}

		logoBoxHeight := contentHeight - components.SingleLineHeight // 1 line for the server text at the bottom
		logoBox := lipgloss.Place(contentWidth, logoBoxHeight, lipgloss.Center, lipgloss.Center, logoContent)
		innerContent = lipgloss.JoinVertical(lipgloss.Center, logoBox, serverPortContent)
	}

	return renderBtopBox("", PaneTitleStyle.Render(" Server "), innerContent, width, height, colors.Gray())
}
