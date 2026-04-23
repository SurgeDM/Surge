package utils

import (
	"strings"
	"unicode"
)

// WrapText wraps a string to a specified maximum width.
// It tries to wrap at word boundaries (spaces).
func WrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		var currentLine strings.Builder
		words := strings.FieldsFunc(line, unicode.IsSpace)

		for _, word := range words {
			// If adding the word plus a space exceeds width
			if currentLine.Len()+len(word)+1 > width {
				if currentLine.Len() > 0 {
					result = append(result, currentLine.String())
					currentLine.Reset()
				}

				// Handle words longer than width by hard-wrapping them
				for len(word) > width {
					result = append(result, word[:width])
					word = word[width:]
				}
				currentLine.WriteString(word)
			} else {
				if currentLine.Len() > 0 {
					currentLine.WriteByte(' ')
				}
				currentLine.WriteString(word)
			}
		}

		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	return strings.Join(result, "\n")
}
