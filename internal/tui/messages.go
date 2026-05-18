package tui

import (
	"github.com/SurgeDM/Surge/internal/version"
)

// notificationTickMsg is sent to check if a notification should be cleared
type notificationTickMsg struct{}

// UpdateCheckResultMsg is sent when the update check is complete
type UpdateCheckResultMsg struct {
	Info *version.UpdateInfo
}

type shutdownCompleteMsg struct {
	err error
}

type enqueueSuccessMsg struct {
	tempID   string
	id       string
	url      string
	path     string
	filename string
}

type enqueueErrorMsg struct {
	tempID string
	err    error
}

type resumeResultMsg struct {
	id  string
	err error
}

// startupConfigWarningMsg carries config validation warnings to display in the
// activity log during Init, after the viewport is sized and ready.
type startupConfigWarningMsg []string
