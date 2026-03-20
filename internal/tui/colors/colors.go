package colors

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
)

// === Color Palette ===
// Vibrant "Cyberpunk" Neon Colors (Dark Mode) + High Contrast (Light Mode)
var (
	NeonPurple color.Color = lipgloss.Color("#5d40c9")
	NeonPink   color.Color = lipgloss.Color("#d10074")
	NeonCyan   color.Color = lipgloss.Color("#0073a8")
	DarkGray   color.Color = lipgloss.Color("#ffffff") // Background
	Gray       color.Color = lipgloss.Color("#d0d0d0") // Borders
	LightGray  color.Color = lipgloss.Color("#4a4a4a") // Brighter text for secondary info
	White      color.Color = lipgloss.Color("#1a1a1a")
)

// === Semantic State Colors ===
var (
	StateError       color.Color = lipgloss.Color("#d32f2f") // Red - Error/Stopped
	StatePaused      color.Color = lipgloss.Color("#f57c00") // Orange - Paused/Queued
	StateDownloading color.Color = lipgloss.Color("#2e7d32") // Green - Downloading
	StateDone        color.Color = lipgloss.Color("#7b1fa2") // Purple - Completed
)

// === Progress Bar Colors ===
var (
	ProgressStart color.Color = lipgloss.Color("#d10074") // Pink
	ProgressEnd   color.Color = lipgloss.Color("#7b1fa2") // Purple
)

var (
	darkMode bool
	modeMu   sync.RWMutex
)

// SetDarkMode applies the active light/dark palette for all exported colors.
func SetDarkMode(isDark bool) {
	modeMu.Lock()
	defer modeMu.Unlock()
	darkMode = isDark

	NeonPurple = themeColorLocked("#5d40c9", "#bd93f9")
	NeonPink = themeColorLocked("#d10074", "#ff79c6")
	NeonCyan = themeColorLocked("#0073a8", "#8be9fd")
	DarkGray = themeColorLocked("#ffffff", "#282a36")
	Gray = themeColorLocked("#d0d0d0", "#44475a")
	LightGray = themeColorLocked("#4a4a4a", "#a9b1d6")
	White = themeColorLocked("#1a1a1a", "#f8f8f2")

	StateError = themeColorLocked("#d32f2f", "#ff5555")
	StatePaused = themeColorLocked("#f57c00", "#ffb86c")
	StateDownloading = themeColorLocked("#2e7d32", "#50fa7b")
	StateDone = themeColorLocked("#7b1fa2", "#bd93f9")

	ProgressStart = themeColorLocked("#d10074", "#ff79c6")
	ProgressEnd = themeColorLocked("#7b1fa2", "#bd93f9")
}

// IsDarkMode reports the current color mode used by the palette.
func IsDarkMode() bool {
	modeMu.RLock()
	defer modeMu.RUnlock()
	return darkMode
}

// ThemeColor returns the light or dark variant based on current mode.
func ThemeColor(lightHex, darkHex string) color.Color {
	modeMu.RLock()
	defer modeMu.RUnlock()
	return themeColorLocked(lightHex, darkHex)
}

func themeColorLocked(lightHex, darkHex string) color.Color {
	if darkMode {
		return lipgloss.Color(darkHex)
	}
	return lipgloss.Color(lightHex)
}
