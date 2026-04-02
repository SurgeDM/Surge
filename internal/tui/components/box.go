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
		remainingWidth := innerWidth - leftTitleWidth - rightTitleWidth - 1 // 1 for the start dash
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
		remainingWidth := innerWidth - leftTitleWidth - 1
		if remainingWidth < 0 {
			remainingWidth = 0
		}

		topBorder = borderStyler.Render(topLeft+horizontal) +
			leftTitle +
			borderStyler.Render(strings.Repeat(horizontal, remainingWidth)+topRight)

	} else if rightTitle != "" {
		// Case 3: Only Right Title
		remainingWidth := innerWidth - rightTitleWidth - 1
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
	bottomBorder := lipgloss.NewStyle().Foreground(borderColor).Render(
		bottomLeft + strings.Repeat(horizontal, innerWidth) + bottomRight,
	)

	// Style for vertical borders
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Wrap content lines with vertical borders
	contentLines := strings.Split(content, "\n")
	innerHeight := height - 2 // Account for top and bottom borders

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
			// Truncate (simplified - just take first innerWidth chars)
			runes := []rune(line)
			if len(runes) > innerWidth {
				line = string(runes[:innerWidth])
			}
		}
		wrappedLines = append(wrappedLines, borderStyle.Render(vertical)+line+borderStyle.Render(vertical))
	}

	return lipgloss.JoinVertical(lipgloss.Left, topBorder, strings.Join(wrappedLines, "\n"), bottomBorder)
}

// Default colors for convenience (re-exported from colors package)
var (
	DefaultBorderColor = colors.NeonPink
	SecondaryBorder    = colors.DarkGray
	AccentBorder       = colors.NeonCyan
)
