package colors

import (
	"image/color"
	"sync"

	"charm.land/lipgloss/v2"
)

type themeColor struct {
	light string
	dark  string
}

func (c themeColor) RGBA() (r, g, b, a uint32) {
	chosen := c.light
	if IsDarkMode() {
		chosen = c.dark
	}
	return lipgloss.Color(chosen).RGBA()
}

// TODO: Load custom colour scheme like Alacritty

// TODO: Rename to use X11/Alacritty colour names

// TODO: Use a TOML spec similar to Alacritty's

// Support either [colors] is_dark = true/false (single colour scheme in fiile) or [colors.dark.] and [colors.light.] in the same file

// === Color Palette ===
// Vibrant "Cyberpunk" Neon Colors (Dark Mode) + High Contrast (Light Mode)
var (
	NeonPurple color.Color = themeColor{light: "#5d40c9", dark: "#bd93f9"}
	NeonPink   color.Color = themeColor{light: "#d10074", dark: "#ff79c6"}
	NeonCyan   color.Color = themeColor{light: "#0073a8", dark: "#8be9fd"}
	DarkGray   color.Color = themeColor{light: "#ffffff", dark: "#282a36"} // Background
	Gray       color.Color = themeColor{light: "#d0d0d0", dark: "#44475a"} // Borders
	LightGray  color.Color = themeColor{light: "#4a4a4a", dark: "#a9b1d6"} // Brighter text for secondary info
	White      color.Color = themeColor{light: "#1a1a1a", dark: "#f8f8f2"}
)

// === Semantic State Colors ===
var (
	StateError       color.Color = themeColor{light: "#d32f2f", dark: "#ff5555"} // Red - Error/Stopped
	StatePaused      color.Color = themeColor{light: "#f57c00", dark: "#ffb86c"} // Orange - Paused/Queued
	StateDownloading color.Color = themeColor{light: "#2e7d32", dark: "#50fa7b"} // Green - Downloading
	StateDone        color.Color = themeColor{light: "#7b1fa2", dark: "#bd93f9"} // Purple - Completed
)

// === Progress Bar Colors ===
var (
	ProgressStart color.Color = themeColor{light: "#950053", dark: "#fa70bc"} // Muted Pink
	ProgressEnd   color.Color = themeColor{light: "#5a1376", dark: "#b472ff"} // Muted Purple
)

var (
	darkMode bool
	modeMu   sync.RWMutex
	hooks    []func()
	hookMu   sync.RWMutex
)

// RegisterThemeChangeHook registers a callback that runs after theme mode flips.
func RegisterThemeChangeHook(fn func()) {
	if fn == nil {
		return
	}
	hookMu.Lock()
	hooks = append(hooks, fn)
	hookMu.Unlock()
}

// SetDarkMode updates the active theme mode and notifies registered listeners.
func SetDarkMode(isDark bool) {
	modeMu.Lock()
	changed := darkMode != isDark
	darkMode = isDark
	modeMu.Unlock()

	if !changed {
		return
	}

	hookMu.RLock()
	registeredHooks := append([]func(){}, hooks...)
	hookMu.RUnlock()
	for _, fn := range registeredHooks {
		fn()
	}
}

// IsDarkMode reports the current color mode used by the palette.
func IsDarkMode() bool {
	modeMu.RLock()
	defer modeMu.RUnlock()
	return darkMode
}

// ThemeColor returns the light or dark variant based on current mode.
// `light` and `dark` accept any Lip Gloss color format (hex, ANSI number, etc.).
func ThemeColor(light, dark string) color.Color {
	return themeColor{light: light, dark: dark}
}
