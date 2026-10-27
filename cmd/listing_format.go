package cmd

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// External filenames and server metadata are text, never terminal commands.
// JSON keeps the original values; only human presentation is sanitized.
func safeTerminalText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, ansi.Strip(value))
}
