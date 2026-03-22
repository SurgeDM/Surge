package components

import (
	"image/color"
	"sync"

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

var (
	statusRenderCache  [StatusError + 1][2]string // [status][0:full,1:icon]
	queuedSpinnerStyle lipgloss.Style
	cacheMu            sync.RWMutex
)

func init() {
	rebuildStatusCache()
	colors.RegisterThemeChangeHook(rebuildStatusCache)
}

func rebuildStatusCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	for status := StatusQueued; status <= StatusError; status++ {
		info := statusMap[status]
		style := lipgloss.NewStyle().Foreground(status.Color())
		statusRenderCache[status][0] = style.Render(info.icon + " " + info.label)
		statusRenderCache[status][1] = style.Render(info.icon)
	}
	queuedSpinnerStyle = lipgloss.NewStyle().Foreground(StatusQueued.Color())
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
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if s >= StatusQueued && s <= StatusError {
		return statusRenderCache[s][0]
	}
	return "Unknown"
}

// RenderWithSpinner returns the styled icon + label combination, conditionally substituting a dynamic spinner for the Queued state
func (s DownloadStatus) RenderWithSpinner(spinnerView string) string {
	if s == StatusQueued {
		cacheMu.RLock()
		style := queuedSpinnerStyle
		cacheMu.RUnlock()
		return style.Render(spinnerView + " " + s.Label())
	}
	return s.Render()
}

// RenderIcon returns just the styled icon
func (s DownloadStatus) RenderIcon() string {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if s >= StatusQueued && s <= StatusError {
		return statusRenderCache[s][1]
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
