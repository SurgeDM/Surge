package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
)

func getXDGBaseDir(envKey, fallback string) string {
	if dir := strings.TrimSpace(os.Getenv(envKey)); dir != "" {
		if filepath.IsAbs(dir) {
			return dir
		}
	}
	return fallback
}

// GetSurgeDir returns the directory for configuration files (settings.json).
// Linux: $XDG_CONFIG_HOME/surge or ~/.config/surge
// macOS: ~/Library/Application Support/surge
// Windows: %APPDATA%/surge
func GetSurgeDir() string {
	if runtime.GOOS == "windows" {
		// Preserve legacy location for existing Windows installs.
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			if filepath.IsAbs(appData) {
				return filepath.Join(appData, "surge")
			}
		}
	}
	return filepath.Join(getXDGBaseDir("XDG_CONFIG_HOME", xdg.ConfigHome), "surge")
}

func GetStateDir() string {
	// Keep state co-located with config on Windows for backward compatibility.
	if runtime.GOOS == "windows" {
		return GetSurgeDir()
	}
	return filepath.Join(getXDGBaseDir("XDG_STATE_HOME", xdg.StateHome), "surge")
}

func GetDownloadsDir() string {
	// Prefer XDG/user-dirs value when it points to a real directory.
	if dir := strings.TrimSpace(xdg.UserDirs.Download); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	// Fallback to ~/Downloads only if it exists.
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		fallback := filepath.Join(home, "Downloads")
		if info, err := os.Stat(fallback); err == nil && info.IsDir() {
			return fallback
		}
	}

	// Final fallback: empty means "current directory" in existing callers.
	return ""
}

func GetRuntimeDir() string {
	runtimeEnv := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeEnv != "" && !filepath.IsAbs(runtimeEnv) {
		runtimeEnv = ""
	}

	runtimeBase := runtimeEnv
	if runtimeBase == "" {
		runtimeBase = strings.TrimSpace(xdg.RuntimeDir)
		if runtimeBase != "" && !filepath.IsAbs(runtimeBase) {
			runtimeBase = ""
		}
	}

	// In headless Linux sessions and Android/Termux, XDG_RUNTIME_DIR is often unset
	// and xdg.RuntimeDir may point to /run/user/<uid>, which can be absent/unwritable.
	// Use a writable state-dir fallback in that case.
	if (runtime.GOOS == "linux" || runtime.GOOS == "android") && runtimeEnv == "" {
		runtimeBase = ""
	}

	if runtimeBase == "" {
		return filepath.Join(GetStateDir(), "runtime")
	}

	return filepath.Join(runtimeBase, "surge")
}

func GetDocumentsDir() string {
	return xdg.UserDirs.Documents
}

func GetMusicDir() string {
	return xdg.UserDirs.Music
}

func GetVideosDir() string {
	return xdg.UserDirs.Videos
}

func GetPicturesDir() string {
	return xdg.UserDirs.Pictures
}

// GetLogsDir returns the directory for logs
func GetLogsDir() string {
	return filepath.Join(GetStateDir(), "logs")
}

// GetThemesDir returns the directory for themes
func GetThemesDir() string {
	return filepath.Join(GetSurgeDir(), "themes")
}

// EnsureDirs creates all required directories
func EnsureDirs() error {
	dirs := []string{GetSurgeDir(), GetStateDir(), GetRuntimeDir(), GetLogsDir(), GetThemesDir()}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// On Linux/Android runtime dir, we might want stricter permissions (0700) if it's in /run/user
	if (runtime.GOOS == "linux" || runtime.GOOS == "android") && os.Getenv("XDG_RUNTIME_DIR") != "" {
		_ = os.Chmod(GetRuntimeDir(), 0o700)
	}

	return nil
}
