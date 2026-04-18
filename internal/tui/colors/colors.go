package colors

import (
	"image/color"
	"strings"
	"path/filepath"
	"sync"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"
)

type Palette struct {
	Name string `toml:"name"`
	Primary struct {
		Background string `toml:"background"`
		Foreground string `toml:"foreground"`
	} `toml:"primary"`
	Normal struct {
		Black   string `toml:"black"`
		Red     string `toml:"red"`
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"`
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	} `toml:"normal"`
	Bright struct {
		Black   string `toml:"black"` // Used for LightGray/Secondary info
		Red     string `toml:"red"`   // Used for ProgressStart (Pink)
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"` // Used for ProgressEnd
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	} `toml:"bright"`
}

type ThemeConfig struct {
    IsDark bool `toml:"is_dark"`
    Colors struct {
        Dark  *Palette `toml:"dark"`  // [colors.dark]
        Light *Palette `toml:"light"` // [colors.light]
        *Palette                      // embedded for single [colors] files
    } `toml:"colors"`
}

var (
	currentPalette *Palette
	isDarkMode bool
	modeMu   sync.RWMutex
	hooks    []func()
	hookMu   sync.RWMutex
)

var defaultDark = Palette{
	Primary: struct {
		Background string `toml:"background"`
		Foreground string `toml:"foreground"`
	}{Background: "#282a36", Foreground: "#f8f8f2"},

	Normal: struct {
		Black   string `toml:"black"`
		Red     string `toml:"red"`
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"`
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	}{Black: "#44475a", Red: "#ff5555", Green: "#50fa7b", Yellow: "#ffb86c", Blue: "#58a6ff", Magenta: "#bd93f9", Cyan: "#8be9fd", White: "#f8f8f2"},

	Bright: struct {
		Black   string `toml:"black"`
		Red     string `toml:"red"`
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"`
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	}{Black: "#a9b1d6", Red: "#ff79c6", Green: "#50fa7b", Yellow: "#ffb86c", Blue: "#58a6ff", Magenta: "#bd93f9", Cyan: "#8be9fd", White: "#f8f8f2"},
}

var defaultLight = Palette{
	Primary: struct {
		Background string `toml:"background"`
		Foreground string `toml:"foreground"`
	}{Background: "#ffffff", Foreground: "#1a1a1a"},

	Normal: struct {
		Black   string `toml:"black"`
		Red     string `toml:"red"`
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"`
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	}{Black: "#d0d0d0", Red: "#d32f2f", Green: "#2e7d32", Yellow: "#f57c00", Blue: "#005cc5", Magenta: "#7b1fa2", Cyan: "#0073a8", White: "#1a1a1a"},

	Bright: struct {
		Black   string `toml:"black"`
		Red     string `toml:"red"`
		Green   string `toml:"green"`
		Yellow  string `toml:"yellow"`
		Blue    string `toml:"blue"`
		Magenta string `toml:"magenta"`
		Cyan    string `toml:"cyan"`
		White   string `toml:"white"`
	}{Black: "#4a4a4a", Red: "#d10074", Green: "#2e7d32", Yellow: "#f57c00", Blue: "#005cc5", Magenta: "#7b1fa2", Cyan: "#0073a8", White: "#1a1a1a"},
}

func init() {
	currentPalette = &defaultDark
	isDarkMode = true
}

func resolveThemePath(path string) string {
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	if !strings.HasSuffix(path, ".toml") {
		pathWithExt := path + ".toml"
		if _, err := os.Stat(pathWithExt); err == nil {
			return pathWithExt
		}
	}

	if _, err := os.Stat(path); err == nil {
		return path
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		xdgPath := filepath.Join(configDir, "surge", "themes", path)
		if _, err := os.Stat(xdgPath); err == nil {
			return xdgPath
		}
	}

	return path
}

func LoadTheme(path string, darkPreferred bool) {
	modeMu.Lock()
	isDarkMode = darkPreferred

	// Start with internal defaults
	newPalette := &defaultLight
	if darkPreferred {
		newPalette = &defaultDark
	}

	resolvedPath := resolveThemePath(path)

	// Only attempt to read if a path was actually provided and resolved
	if resolvedPath != "" {
		if data, err := os.ReadFile(resolvedPath); err == nil {
			var cfg ThemeConfig
			if err := toml.Unmarshal(data, &cfg); err == nil {
				// 1. Priority: Specific [colors.dark] or [colors.light] blocks
				if darkPreferred && cfg.Colors.Dark != nil {
					newPalette = cfg.Colors.Dark
				} else if !darkPreferred && cfg.Colors.Light != nil {
					newPalette = cfg.Colors.Light
				} else if cfg.Colors.Palette != nil {
					// 2. Fallback: The embedded [colors] block for single-scheme files
					// Note: With embedding, we check if the pointer inside the struct is non-nil
					newPalette = cfg.Colors.Palette
				}
			}
		}
	}

	currentPalette = newPalette
	modeMu.Unlock()
	triggerHooks()
}

func triggerHooks() {
	hookMu.RLock()
	registeredHooks := append([]func(){}, hooks...)
	hookMu.RUnlock()
	for _, fn := range registeredHooks {
		fn()
	}
}

func palette() *Palette {
    modeMu.RLock()
    p := currentPalette
    modeMu.RUnlock()
    return p
}

func Background() color.Color { return lipgloss.Color(palette().Primary.Background) }
func Foreground() color.Color { return lipgloss.Color(palette().Primary.Foreground) }

// Semantic Mappings
func White() color.Color { return lipgloss.Color(palette().Normal.White) }
func Gray() color.Color { return lipgloss.Color(palette().Normal.Black) }
func Red() color.Color  { return lipgloss.Color(palette().Normal.Red) }
func Pink() color.Color  { return lipgloss.Color(palette().Bright.Red) }
func Green() color.Color { return lipgloss.Color(palette().Normal.Green) }
func Orange() color.Color { return lipgloss.Color(palette().Normal.Yellow) }
func Blue() color.Color { return lipgloss.Color(palette().Normal.Blue) }
func Magenta() color.Color { return lipgloss.Color(palette().Normal.Magenta) }
func Cyan() color.Color { return lipgloss.Color(palette().Normal.Cyan) }
func LightGray() color.Color { return lipgloss.Color(palette().Bright.Black) }
func DarkGray() color.Color { return lipgloss.Color(palette().Bright.Black) }

// State Mappings
func StateError() color.Color       { return Red() }
func StatePaused() color.Color      { return Orange() }
func StateDownloading() color.Color { return Green() }
func StateDone() color.Color        { return Magenta() }
func StateVersion() color.Color     { return Blue() }

// Progress Mappings
func ProgressStart() color.Color { return lipgloss.Color(palette().Bright.Red) } // Neon Pink
func ProgressEnd() color.Color   { return lipgloss.Color(palette().Bright.Magenta) }

type themeColor struct {
	light string
	dark  string
}

func IsDarkMode() bool {
	modeMu.RLock()
	defer modeMu.RUnlock()
	return isDarkMode
}

func (c themeColor) RGBA() (r, g, b, a uint32) {
	chosen := c.light
	if IsDarkMode() {
		chosen = c.dark
	}
	return lipgloss.Color(chosen).RGBA()
}

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
	changed := isDarkMode != isDark
	isDarkMode = isDark
	if isDark {
		currentPalette = &defaultDark
	} else {
		currentPalette = &defaultLight
	}
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

// ThemeColor returns the light or dark variant based on current mode.
// `light` and `dark` accept any Lip Gloss color format (hex, ANSI number, etc.).
func ThemeColor(light, dark string) color.Color {
	return themeColor{light: light, dark: dark}
}
