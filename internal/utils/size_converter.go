package utils

import "fmt"

var sizes = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

// ConvertBytesToHumanReadable converts a given number of bytes into a human-readable format (e.g., KB, MB, GB).
func ConvertBytesToHumanReadable(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}

	base := 1000.0
	val := float64(bytes)
	i := 0
	for val >= base && i < len(sizes)-1 {
		val /= base
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%d B", bytes)
	}

	if val < 10 {
		return fmt.Sprintf("%.1f %s", val, sizes[i])
	}
	return fmt.Sprintf("%.0f %s", val, sizes[i])
}
