package tui

import (
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/version"
)

// UpdateCheckResultMsg is sent when the update check is complete
type UpdateCheckResultMsg struct {
	Info *version.UpdateInfo
}

type selfUpdateResultMsg struct {
	Info *version.UpdateInfo
	err  error
}

type shutdownCompleteMsg struct {
	err error
}

type autoShutdownResultMsg struct {
	err error
}

type autoShutdownCheckMsg struct{}

type autoShutdownRetryMsg struct{}

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

type resyncDownloadsMsg struct {
	revision uint64
	statuses []types.DownloadStatus
	err      error
}

// A delayed retry belongs to one recovery cycle, not a lifecycle revision:
// intervening lifecycle events must be included in its fresh snapshot.
type resyncRetryMsg struct{ generation uint64 }
