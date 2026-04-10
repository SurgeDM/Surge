package components

import (
	"image/color"
	"strings"

	"github.com/SurgeDM/Surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

// BoxRenderer is the function signature for rendering btop-style boxes
type BoxRenderer func(leftTitle, rightTitle, content string, width, height int, borderColor color.Color) string

// RenderBtopBox creates a btop-style box with title embedded in the top border.
// Supports left and right titles (e.g., search on left, pane name on right).
// Accepts pre-styled title strings.
// Example: ╭─ 🔍 Search... ─────────── Downloads ─╮
func RenderBtopBox(leftTitle, rightTitle string, content string, width, height int, borderColor color.Color) string {
	// Border characters
	const (
		topLeft     = "╭"
		topRight    = "╮"
		bottomLeft  = "╰"
		bottomRight = "╯"
		horizontal  = "─"
		vertical    = "│"
	)
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	leftTitleWidth := lipgloss.Width(leftTitle)
	rightTitleWidth := lipgloss.Width(rightTitle)

	// Calculate remaining horizontal space for the border
	// Structure: ╭ + horizontal*? + leftTitle + horizontal*? + rightTitle + horizontal*? + ╮
	// Basic structure we want:
	// If leftTitle exists: ╭─ leftTitle ──...
	// If rightTitle exists: ...── rightTitle ─╮

	borderStyler := lipgloss.NewStyle().Foreground(borderColor)
	var topBorder string

	// Case 1: Both Titles
	if leftTitle != "" && rightTitle != "" {
		remainingWidth := innerWidth - leftTitleWidth - rightTitleWidth - lipgloss.Width(horizontal)
		if remainingWidth < 1 {
			remainingWidth = 1 // overflow mitigation (might break layout but prevents crash)
		}

		topBorder = borderStyler.Render(topLeft+horizontal) +
			leftTitle +
			borderStyler.Render(strings.Repeat(horizontal, remainingWidth)) +
			rightTitle +
			borderStyler.Render(topRight)

	} else if leftTitle != "" {
		// Case 2: Only Left Title
		remainingWidth := innerWidth - leftTitleWidth - lipgloss.Width(horizontal)
		if remainingWidth < 0 {
			remainingWidth = 0
		}

		topBorder = borderStyler.Render(topLeft+horizontal) +
			leftTitle +
			borderStyler.Render(strings.Repeat(horizontal, remainingWidth)+topRight)

	} else if rightTitle != "" {
		// Case 3: Only Right Title
		remainingWidth := innerWidth - rightTitleWidth - lipgloss.Width(horizontal)
		if remainingWidth < 0 {
			remainingWidth = 0
		}

		topBorder = borderStyler.Render(topLeft+strings.Repeat(horizontal, remainingWidth)) +
			rightTitle +
			borderStyler.Render(horizontal+topRight)

	} else {
		// Case 4: No Title
		topBorder = borderStyler.Render(topLeft + strings.Repeat(horizontal, innerWidth) + topRight)
	}

	// Build bottom border: ╰───────────────────╯
	bottomBorder := borderStyler.Render(
		bottomLeft + strings.Repeat(horizontal, innerWidth) + bottomRight,
	)

	// Wrap content lines with vertical borders
	contentLines := strings.Split(content, "\n")
	innerHeight := height - 2 // Account for top and bottom borders

	// Style for truncation
	truncStyle := lipgloss.NewStyle().MaxWidth(innerWidth)

	var wrappedLines []string
	for i := 0; i < innerHeight; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
		} else {
			line = ""
		}
		// Pad or truncate line to fit innerWidth
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			line = line + strings.Repeat(" ", innerWidth-lineWidth)
		} else if lineWidth > innerWidth {
			line = truncStyle.Render(line)
		}
		wrappedLines = append(wrappedLines, borderStyler.Render(vertical)+line+borderStyler.Render(vertical))
	}

	return lipgloss.JoinVertical(lipgloss.Left, topBorder, strings.Join(wrappedLines, "\n"), bottomBorder)
}

// Default colors for convenience (re-exported from colors package)
var (
	DefaultBorderColor = colors.NeonPink
	SecondaryBorder    = colors.DarkGray
	AccentBorder       = colors.NeonCyan
)
