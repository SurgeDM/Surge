package components

import (
	"image/color"

	"github.com/surge-downloader/surge/internal/tui/colors"

	"charm.land/lipgloss/v2"
)

// DownloadStatus represents the state of a download
type DownloadStatus int

const (
	StatusQueued DownloadStatus = iota
	StatusDownloading
	StatusPaused
	StatusComplete
	StatusError
)

// statusInfo holds the display properties for each status
type statusInfo struct {
	icon  string
	label string
}

var statusMap = map[DownloadStatus]statusInfo{
	StatusQueued:      {icon: "⋯", label: "Queued"},
	StatusDownloading: {icon: "⬇", label: "Downloading"},
	StatusPaused:      {icon: "⏸", label: "Paused"},
	StatusComplete:    {icon: "✔", label: "Completed"},
	StatusError:       {icon: "✖", label: "Error"},
}

// Icon returns the status icon
func (s DownloadStatus) Icon() string {
	if info, ok := statusMap[s]; ok {
		return info.icon
	}
	return "?"
}

// Label returns the status label
func (s DownloadStatus) Label() string {
	if info, ok := statusMap[s]; ok {
		return info.label
	}
	return "Unknown"
}

// Color returns the status color
func (s DownloadStatus) Color() color.Color {
	switch s {
	case StatusQueued, StatusPaused:
		return colors.StatePaused
	case StatusDownloading:
		return colors.StateDownloading
	case StatusComplete:
		return colors.StateDone
	case StatusError:
		return colors.StateError
	default:
		return colors.Gray
	}
}

// Render returns the styled icon + label combination
func (s DownloadStatus) Render() string {
	if info, ok := statusMap[s]; ok {
		return lipgloss.NewStyle().Foreground(s.Color()).Render(info.icon + " " + info.label)
	}
	return "Unknown"
}

// RenderIcon returns just the styled icon
func (s DownloadStatus) RenderIcon() string {
	if info, ok := statusMap[s]; ok {
		return lipgloss.NewStyle().Foreground(s.Color()).Render(info.icon)
	}
	return "?"
}

// DetermineStatus determines the DownloadStatus based on download state
// This centralizes the status determination logic that was duplicated in view.go and list.go
func DetermineStatus(done bool, paused bool, hasError bool, speed float64, downloaded int64) DownloadStatus {
	switch {
	case hasError:
		return StatusError
	case done:
		return StatusComplete
	case paused:
		return StatusPaused
	case speed == 0 && downloaded == 0:
		return StatusQueued
	default:
		return StatusDownloading
	}
}
