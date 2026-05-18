package types

import "errors"

// Common errors
var (
	ErrPaused             = errors.New("download paused")
	ErrNotFound           = errors.New("download not found")
	ErrCompleted          = errors.New("download already completed")
	ErrPausing            = errors.New("download is still pausing, try again in a moment")
	ErrEngineNotInit      = errors.New("engine not initialized")
	ErrPoolNotInit        = errors.New("worker pool not initialized")
	ErrIDExists           = errors.New("download id already exists")
	ErrURLRequired        = errors.New("URL is required")
	ErrDestRequired       = errors.New("destination path is required")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrQueuedUpdate       = errors.New("cannot update URL for a queued download, please cancel or wait for it to start")
	ErrActiveUpdate       = errors.New("download is currently active, please pause it before updating the URL")
	ErrMaxRedirects       = errors.New("stopped after 10 redirects")
)
