package utils

import (
	"math"

	"github.com/dustin/go-humanize"
)

const (
	B   = 1
	KiB = 1 << 10
	MiB = 1 << 20
	GiB = 1 << 30
	TiB = 1 << 40
)

// FormatBytes formats a byte count into a human-readable string using the IEC standard (e.g., KiB, MiB, GiB).
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		return "-" + humanize.IBytes(uint64(-bytes))
	}
	return humanize.IBytes(uint64(bytes))
}

// FormatSpeed formats a live speed value (e.g., bytes per second) into an IEC standard string.
func FormatSpeed(speedBps float64) string {
	if speedBps <= 0 {
		return "0 B/s"
	}
	return FormatBytes(int64(math.Round(speedBps))) + "/s"
}
