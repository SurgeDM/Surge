package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
)

// renderChunkMapBox returns the visual chunk map layout inside a btop box.
func (m *RootModel) renderChunkMapBox(width, height int, selected *DownloadModel, bitmap []byte, bitmapWidth int, totalSize, chunkSize int64, chunkProgress []int64) string {
	contentWidth := width - components.BorderFrameWidth
	contentHeight := height - components.BorderFrameHeight

	if contentWidth < 0 {
		contentWidth = 0
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	var innerContent string
	if len(bitmap) == 0 || bitmapWidth == 0 {
		innerContent = renderEmptyMessage(contentWidth, contentHeight, "Chunk visualization not available")
	} else {

		targetRows := contentHeight
		if targetRows < 3 {
			targetRows = 3
		}
		if targetRows > 5 {
			targetRows = 5 // Maximum 5 rows for compact look
		}

		chunkMapPadding := lipgloss.NewStyle().Padding(0, 2)
		chunkMapContentWidth := contentWidth - chunkMapPadding.GetHorizontalFrameSize()
		if chunkMapContentWidth < 4 {
			chunkMapContentWidth = 4
		}

		paused := false
		if selected != nil {
			paused = selected.paused
		}

		chunkMap := components.NewChunkMapModel(bitmap, bitmapWidth, chunkMapContentWidth, targetRows, paused, totalSize, chunkSize, chunkProgress)
		chunkContentWrapper := chunkMapPadding.Render(chunkMap.View())

		innerContent = lipgloss.Place(contentWidth, contentHeight, lipgloss.Center, lipgloss.Top, chunkContentWrapper)

	}

	return renderBtopBox("", PaneTitleStyle.Render(" Chunk Map "), innerContent, width, height, colors.Gray)
}
