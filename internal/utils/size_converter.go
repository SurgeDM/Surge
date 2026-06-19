package utils

import "fmt"

// ConvertBytesToHumanReadable converts a given number of bytes into a human-readable format (e.g., kB, MB, GB).
func ConvertBytesToHumanReadable(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}

	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "kMGTPE"[exp])
}
