package types

import "errors"

// Common errors
var (
	ErrPaused   = errors.New("download paused")
	ErrNotFound = errors.New("download not found")
)
