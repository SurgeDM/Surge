package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/utils"
)

// renderGraphBox returns the network activity sparkline box layout.
func (m *RootModel) renderGraphBox(width, height int, stats ViewStats) string {
	if width < 1 || height < 1 {
		return ""
	}

	contentWidth := width - 2
	contentHeight := height - 2

	if contentWidth < 0 {
		contentWidth = 0
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Calculate Available Height for the Graph
	// Let's leave 2 lines for top/bottom padding as previous design.
	graphContentHeight := contentHeight - 2
	if graphContentHeight < 3 {
		graphContentHeight = 3
	}

	// Determine if we should hide stats box
	hideGraphStats := width < MinGraphStatsWidth

	// Get the last data points for the graph
	var graphData []float64
	if len(m.SpeedHistory) > GraphHistoryPoints {
		graphData = m.SpeedHistory[len(m.SpeedHistory)-GraphHistoryPoints:]
	} else {
		graphData = m.SpeedHistory
	}

	// Determine Max Speed for scaling
	maxSpeed := 0.0
	topSpeed := 0.0
	for _, v := range graphData {
		if v > maxSpeed {
			maxSpeed = v
		}
		if v > topSpeed {
			topSpeed = v
		}
	}

	if maxSpeed == 0 {
		maxSpeed = 1.0 // Default scale for empty graph
	} else {
		// Add headroom
		maxSpeed = maxSpeed * GraphHeadroom
		if maxSpeed < 1.0 {
			maxSpeed = 1.0
		}
		if maxSpeed >= 5 {
			maxSpeed = float64(int((maxSpeed+4.99)/5) * 5)
		} else {
			maxSpeed = float64(int(maxSpeed + 0.99))
		}
	}

	buildAxisLines := func(h int, axisStyle lipgloss.Style) []string {
		label := func(v float64) string {
			if v <= 0 {
				return "0 MB/s"
			}
			return fmt.Sprintf("%.1f MB/s", v)
		}

		axisLines := make([]string, h)
		for i := range axisLines {
			axisLines[i] = axisStyle.Render("")
		}

		type axisMark struct {
			num int
			den int
		}

		marks := []axisMark{
			{num: 1, den: 1},
			{num: 1, den: 2},
			{num: 0, den: 1},
		}
		if h >= 9 {
			marks = []axisMark{
				{num: 1, den: 1},
				{num: 4, den: 5},
				{num: 3, den: 5},
				{num: 2, den: 5},
				{num: 1, den: 5},
				{num: 0, den: 1},
			}
		}

		for _, mark := range marks {
			row := 0
			if h > 1 {
				row = ((mark.den-mark.num)*(h-1) + mark.den/2) / mark.den
			}
			value := maxSpeed * float64(mark.num) / float64(mark.den)
			axisLines[row] = axisStyle.Render(label(value))
		}

		return axisLines
	}

	var graphWithAxis string

	if hideGraphStats {
		// No stats box — graph gets almost full width
		graphAreaWidth, axisWidth := GetGraphAreaDimensions(width, true)
		graphVisual := renderMultiLineGraph(graphData, graphAreaWidth, graphContentHeight, maxSpeed, nil)

		axisStyle := lipgloss.NewStyle().Width(axisWidth).Foreground(colors.NeonCyan).Align(lipgloss.Right)
		axisLines := buildAxisLines(graphContentHeight, axisStyle)
		axisColumn := lipgloss.NewStyle().
			Height(graphContentHeight).
			Align(lipgloss.Right).
			Render(strings.Join(axisLines, "\n"))

		graphWithAxis = lipgloss.JoinHorizontal(lipgloss.Top, graphVisual, axisColumn)
	} else {
		// Get current speed and calculate total downloaded
		currentSpeed := 0.0
		if len(m.SpeedHistory) > 0 {
			currentSpeed = m.SpeedHistory[len(m.SpeedHistory)-1]
		}

		speedMbps := currentSpeed * 8
		topMbps := topSpeed * 8

		valueStyle := lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true)
		labelStyleStats := lipgloss.NewStyle().Foreground(colors.LightGray)
		dimStyle := lipgloss.NewStyle().Foreground(colors.Gray)

		statsContent := lipgloss.JoinVertical(lipgloss.Left,
			fmt.Sprintf("%s %s", valueStyle.Render("\u25bc"), valueStyle.Render(fmt.Sprintf("%.2f MB/s", currentSpeed))),
			dimStyle.Render(fmt.Sprintf("  (%.0f Mbps)", speedMbps)),
			"",
			fmt.Sprintf("%s %s", labelStyleStats.Render("Top:"), valueStyle.Render(fmt.Sprintf("%.2f", topSpeed))),
			dimStyle.Render(fmt.Sprintf("  (%.0f Mbps)", topMbps)),
			"",
			fmt.Sprintf("%s %s", labelStyleStats.Render("Total:"), valueStyle.Render(utils.ConvertBytesToHumanReadable(stats.TotalDownloaded))),
		)

		statsBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Gray).
			Padding(0, 1).
			Width(GraphStatsWidth).
			Height(graphContentHeight)
		statsBox := statsBoxStyle.Render(statsContent)

		graphAreaWidth, axisWidth := GetGraphAreaDimensions(width, false)
		graphVisual := renderMultiLineGraph(graphData, graphAreaWidth, graphContentHeight, maxSpeed, nil)

		axisStyle := lipgloss.NewStyle().Width(axisWidth).Foreground(colors.NeonCyan).Align(lipgloss.Right)
		axisLines := buildAxisLines(graphContentHeight, axisStyle)
		axisColumn := lipgloss.NewStyle().
			Height(graphContentHeight).
			Align(lipgloss.Right).
			Render(strings.Join(axisLines, "\n"))

		graphWithAxis = lipgloss.JoinHorizontal(lipgloss.Top, statsBox, graphVisual, axisColumn)
	}

	innerContent := lipgloss.JoinVertical(lipgloss.Left, "", graphWithAxis, "")
	return renderBtopBox(PaneTitleStyle.Render(" Network Activity "), "", innerContent, width, height, colors.NeonCyan)
}
