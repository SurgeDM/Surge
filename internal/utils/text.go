package utils

import (
	"github.com/charmbracelet/x/ansi"
)

// WrapText wraps a string to a specified maximum width.
// It is ANSI-aware.
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	return ansi.Wrap(text, width, "")
}

// Truncate truncates a string to a maximum visual width and adds an ellipsis if needed.
// It is ANSI-aware.
func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= limit {
		return s
	}
	if limit <= 1 {
		return "\u2026"
	}
	return ansi.Truncate(s, limit-1, "") + "\u2026"
}

// TruncateMiddle truncates a string in the middle to a maximum visual width.
// It is ANSI-aware.
func TruncateMiddle(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	totalW := ansi.StringWidth(s)
	if totalW <= limit {
		return s
	}
	if limit < 3 {
		return Truncate(s, limit)
	}

	leftLimit := (limit - 1) / 2
	rightLimit := limit - 1 - leftLimit
	remove := totalW - rightLimit

	left := ansi.Truncate(s, leftLimit, "")
	right := ansi.TruncateLeft(s, remove, "")
	return left + "\u2026" + right
}

// TruncateTwoLines middle-truncates a string to fit in at most 2 lines of a given width.
// It uses character-based wrapping (ignoring word boundaries) to maximize space usage.
func TruncateTwoLines(s string, width int) string {
	if width <= 0 {
		return s
	}

	// 1. Truncate in the middle if it exceeds 2 lines of visual width
	truncated := TruncateMiddle(s, 2*width)

	// 2. Wrap based on characters (visual width)
	return ansi.Hardwrap(truncated, width, false)
}
