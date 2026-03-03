package utils

import (
	"github.com/dustin/go-humanize"
)

// ConvertBytesToHumanReadable converts a given number of bytes into a human-readable format (e.g., KB, MB, GB).
func ConvertBytesToHumanReadable(bytes int64) string {
	if bytes < 0 {
		return humanize.Bytes(0)
	}
	if bytes == 0 {
		return "0 B"
	}
	// go-humanize uses SI standards (kB, MB) but format uses standard text
	return humanize.Bytes(uint64(bytes))
}
