package tui

import "time"

const (
	// GraphUpdateInterval is the interval at which the speed history is updated (polling rate).
	GraphUpdateInterval = 500 * time.Millisecond

	// GraphHistoryPoints is the number of data points to keep in history
	// 60 points * 0.5s interval = 30 seconds of history.
	GraphHistoryPoints = 60
)
