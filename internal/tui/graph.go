package tui

import (
	"image/color"
	"strings"

	"github.com/SurgeDM/Surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

func graphColors() []color.Color {
	return []color.Color{
		colors.ProgressStart(), // Bottom
		colors.Magenta(),
		colors.Pink(),
		colors.ProgressEnd(), // Top
	}
}

// renderMultiLineGraph creates a multi-line bar graph with grid lines.
// The graph scales data to fill the full width.
// data: speed history data points
// width, height: dimensions of the graph
// maxVal: maximum value for scaling
func renderMultiLineGraph(data []float64, width, height int, maxVal float64) string {
	if width < 1 || height < 1 {
		return ""
	}

	// Styles
	gridStyle := lipgloss.NewStyle().Foreground(colors.Gray())

	// 1. Prepare the canvas with a Grid
	rows := make([][]string, height)
	for i := range rows {
		rows[i] = make([]string, width)
		for j := range rows[i] {
			if i == height-1 {
				// Bottom row: solid baseline
				rows[i][j] = gridStyle.Render("\u2500")
			} else if i%2 == 0 {
				rows[i][j] = gridStyle.Render("\u254c")
			} else {
				rows[i][j] = " "
			}
		}
	}

	// Block characters for partial fills
	blocks := []string{" ", "\u2581", "\u2582", "\u2583", "\u2584", "\u2585", "\u2586", "\u2588"}

	// Snapshot current palette colors once per render so the gradient is consistent
	// across all rows and doesn't allocate on every iteration.
	gradient := graphColors()

	// Pre-calculate styles for every row to avoid re-creating them in the loop
	// Optimization: Pre-render all possible block characters for each row style
	// This avoids calling style.Render() width*height times
	rowChars := make([][]string, height)
	for y := 0; y < height; y++ {
		// Map height 'y' to an index in gradient
		colorIdx := (y * len(gradient)) / height
		if colorIdx >= len(gradient) {
			colorIdx = len(gradient) - 1
		}
		style := lipgloss.NewStyle().Foreground(gradient[colorIdx])

		rowChars[y] = make([]string, len(blocks))
		for k, b := range blocks {
			rowChars[y][k] = style.Render(b)
		}
	}

	// 2. Scale data to fill full width
	// Each data point spans multiple columns to fill the graph
	if len(data) > 0 {
		colsPerPoint := float64(width) / float64(len(data))

		for i, val := range data {
			if val < 0 {
				val = 0
			}

			pct := val / maxVal
			if pct > 1.0 {
				pct = 1.0
			}
			totalSubBlocks := pct * float64(height) * 8.0

			// Calculate column range for this data point
			startCol := int(float64(i) * colsPerPoint)
			endCol := int(float64(i+1) * colsPerPoint)
			if endCol > width {
				endCol = width
			}

			// Draw the bar across all columns for this data point
			for col := startCol; col < endCol; col++ {
				for y := 0; y < height; y++ {
					rowIndex := height - 1 - y
					rowValue := totalSubBlocks - float64(y*8)

					var charIndex int
					if rowValue <= 0 {
						charIndex = 0 // Space
					} else if rowValue >= 8 {
						charIndex = 7 // Full block (█)
					} else {
						charIndex = int(rowValue) // Partial block
					}

					// USE PRE-RENDERED CACHE
					if charIndex > 0 { // Only render if not empty space (optimization)
						rows[rowIndex][col] = rowChars[y][charIndex]
					}
				}
			}
		}
	}

	// 3. Join rows to create the graph
	var graphBuilder strings.Builder
	for i, row := range rows {
		graphBuilder.WriteString(strings.Join(row, ""))
		if i < height-1 {
			graphBuilder.WriteRune('\n')
		}
	}
	graphStr := graphBuilder.String()

	return graphStr
}
