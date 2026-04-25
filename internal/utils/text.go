package utils

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"charm.land/lipgloss/v2"
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
// It is ANSI-aware and will include escape codes without counting them towards width.
func truncateToWidth(s string, width int) string {
	infos := getCharInfos(s)
	var res strings.Builder
	var currentW int
	for i, info := range infos {
		if info.w > 0 && currentW+info.w > width {
			// Check if we need to add a reset
			state := getAnsiState(infos, i)
			if state != "" {
				res.WriteString("\x1b[0m")
			}
			return res.String()
		}
		res.WriteRune(info.r)
		currentW += info.w
	}
	
	return res.String()
}

type charInfo struct {
	r rune
	w int
}

func getCharInfos(s string) []charInfo {
	var infos []charInfo
	inAnsi := false
	for _, r := range s {
		if r == '\x1b' {
			inAnsi = true
		}

		w := 0
		if !inAnsi {
			w = runewidth.RuneWidth(r)
		}

		infos = append(infos, charInfo{r, w})

		// Simple SGR sequence end detection
		if inAnsi && r == 'm' {
			inAnsi = false
		}
	}
	return infos
}

func getAnsiState(infos []charInfo, endIdx int) string {
	var state strings.Builder
	var currentAnsi strings.Builder
	inAnsi := false
	for i := 0; i < endIdx && i < len(infos); i++ {
		r := infos[i].r
		if r == '\x1b' {
			inAnsi = true
			currentAnsi.WriteRune(r)
			continue
		}
		if inAnsi {
			currentAnsi.WriteRune(r)
			if r == 'm' {
				inAnsi = false
				seq := currentAnsi.String()
				if seq == "\x1b[0m" || seq == "\x1b[m" {
					state.Reset()
				} else {
					state.WriteString(seq)
				}
				currentAnsi.Reset()
			}
			continue
		}
	}
	return state.String()
}

func stringWidth(s string) int {
	return lipgloss.Width(s)
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

// TruncateMiddle truncates a string in the middle to a maximum visual width.
// It is ANSI-aware.
func TruncateMiddle(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	totalW := stringWidth(s)
	if totalW <= limit {
		return s
	}
	if limit < 3 {
		return Truncate(s, limit)
	}

	leftLimit := (limit - 1) / 2
	rightLimit := limit - 1 - leftLimit

	infos := getCharInfos(s)
	var left strings.Builder
	currentW := 0
	leftEndIdx := 0
	for i, info := range infos {
		if info.w > 0 && currentW+info.w > leftLimit {
			break
		}
		left.WriteRune(info.r)
		currentW += info.w
		leftEndIdx = i + 1
	}

	var right strings.Builder
	currentW = 0
	rightStartIdx := -1
	for i := len(infos) - 1; i >= 0; i-- {
		info := infos[i]
		if info.w > 0 && currentW+info.w > rightLimit {
			break
		}
		currentW += info.w
		rightStartIdx = i
	}

	if rightStartIdx != -1 {
		for i := rightStartIdx; i < len(infos); i++ {
			right.WriteRune(infos[i].r)
		}
	}

	lStr := left.String()
	state := getAnsiState(infos, leftEndIdx)
	if state != "" {
		if !strings.HasSuffix(lStr, "\x1b[0m") {
			lStr += "\x1b[0m"
		}
		return lStr + "…" + state + right.String()
	}

	return lStr + "…" + right.String()
}

// TruncateTwoLines middle-truncates a string to fit in at most 2 lines of a given width.
// It uses character-based wrapping (ignoring word boundaries) to maximize space usage.
func TruncateTwoLines(s string, width int) string {
	if width <= 0 {
		return s
	}

	// 1. Truncate in the middle if it exceeds 2 lines of visual width
	truncated := TruncateMiddle(s, 2*width)

	// 2. Wrap based on characters (visual width) by building lines rune by rune
	infos := getCharInfos(truncated)
	var lines []string
	var currentLine strings.Builder
	currentW := 0
	
	for _, info := range infos {
		if info.w > 0 && currentW+info.w > width {
			if len(lines) < 1 { // We only need 2 lines max
				lines = append(lines, currentLine.String())
				currentLine.Reset()
				currentW = 0
			} else {
				// We already have one line and this would start a third line
				// So we stop here.
				currentLine.Reset()
				currentW = -1 // Mark as finished
				break
			}
		}
		if currentW != -1 {
			currentLine.WriteRune(info.r)
			currentW += info.w
		}
	}

	if currentW != -1 && currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}
