package colors

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
)

// === Color Palette ===
// Vibrant "Cyberpunk" Neon Colors (Dark Mode) + High Contrast (Light Mode)
var (
	NeonPurple color.Color
	NeonPink   color.Color
	NeonCyan   color.Color
	DarkGray   color.Color // Background
	Gray       color.Color // Borders
	LightGray  color.Color // Brighter text for secondary info
	White      color.Color
)

// === Semantic State Colors ===
var (
	StateError       color.Color // Red - Error/Stopped
	StatePaused      color.Color // Orange - Paused/Queued
	StateDownloading color.Color // Green - Downloading
	StateDone        color.Color // Purple - Completed
)

// === Progress Bar Colors ===
var (
	ProgressStart color.Color // Pink
	ProgressEnd   color.Color // Purple
)

var darkMode bool

func init() {
	SetDarkMode(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
}

// SetDarkMode applies the active light/dark palette for all exported colors.
func SetDarkMode(isDark bool) {
	darkMode = isDark

	NeonPurple = ThemeColor("#5d40c9", "#bd93f9")
	NeonPink = ThemeColor("#d10074", "#ff79c6")
	NeonCyan = ThemeColor("#0073a8", "#8be9fd")
	DarkGray = ThemeColor("#ffffff", "#282a36")
	Gray = ThemeColor("#d0d0d0", "#44475a")
	LightGray = ThemeColor("#4a4a4a", "#a9b1d6")
	White = ThemeColor("#1a1a1a", "#f8f8f2")

	StateError = ThemeColor("#d32f2f", "#ff5555")
	StatePaused = ThemeColor("#f57c00", "#ffb86c")
	StateDownloading = ThemeColor("#2e7d32", "#50fa7b")
	StateDone = ThemeColor("#7b1fa2", "#bd93f9")

	ProgressStart = ThemeColor("#d10074", "#ff79c6")
	ProgressEnd = ThemeColor("#7b1fa2", "#bd93f9")
}

// IsDarkMode reports the current color mode used by the palette.
func IsDarkMode() bool {
	return darkMode
}

// ThemeColor returns the light or dark variant based on current mode.
func ThemeColor(lightHex, darkHex string) color.Color {
	if darkMode {
		return lipgloss.Color(darkHex)
	}
	return lipgloss.Color(lightHex)
}
