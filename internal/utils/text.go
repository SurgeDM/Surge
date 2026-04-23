package utils

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// WrapText wraps a string to a specified maximum width.
// It tries to wrap at word boundaries (spaces) and handles multi-byte runes and visual width.
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if runewidth.StringWidth(line) <= width {
			result = append(result, line)
			continue
		}

		var currentLine strings.Builder
		currentLineWidth := 0
		words := strings.FieldsFunc(line, unicode.IsSpace)

		for _, word := range words {
			wordWidth := runewidth.StringWidth(word)

			// If adding the word plus a space exceeds width
			spaceWidth := 0
			if currentLineWidth > 0 {
				spaceWidth = 1
			}

			if currentLineWidth+wordWidth+spaceWidth > width {
				if currentLineWidth > 0 {
					result = append(result, currentLine.String())
					currentLine.Reset()
				}

				// Handle words longer than width by hard-wrapping them
				for runewidth.StringWidth(word) > width {
					// Find where to break
					sub := truncateToWidth(word, width)
					result = append(result, sub)
					word = word[len(sub):]
				}
				currentLine.WriteString(word)
				currentLineWidth = runewidth.StringWidth(word)
			} else {
				if currentLineWidth > 0 {
					currentLine.WriteByte(' ')
					currentLineWidth++
				}
				currentLine.WriteString(word)
				currentLineWidth += wordWidth
			}
		}

		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	return strings.Join(result, "\n")
}

// truncateToWidth truncates a string to a visual width and returns the truncated string.
func truncateToWidth(s string, width int) string {
	var res strings.Builder
	var w int
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > width {
			break
		}
		res.WriteRune(r)
		w += rw
	}
	return res.String()
}

// Truncate truncates a string to a maximum visual width and adds an ellipsis if needed.
func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= limit {
		return s
	}
	if limit <= 1 {
		return "…"
	}

	sub := truncateToWidth(s, limit-1)
	return sub + "…"
}

// truncateMiddle truncates a string in the middle to a maximum visual width.
func truncateMiddle(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= limit {
		return s
	}
	if limit < 3 {
		return Truncate(s, limit)
	}

	leftLimit := (limit - 1) / 2
	rightLimit := limit - 1 - leftLimit

	left := truncateToWidth(s, leftLimit)

	// For the right part, we need to find how many characters from the end fit in rightLimit
	runes := []rune(s)
	right := ""
	rightWidth := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if rightWidth+rw > rightLimit {
			break
		}
		right = string(runes[i]) + right
		rightWidth += rw
	}

	return left + "…" + right
}
