package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
)

// GetSurgeDir returns the directory for configuration files (settings.json).
// Linux: $XDG_CONFIG_HOME/surge or ~/.config/surge
// macOS: ~/Library/Application Support/surge
// Windows: %APPDATA%/surge
func GetSurgeDir() string {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "surge")
	case "darwin": // MacOS
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "surge")
	default: // Linux
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, _ := os.UserHomeDir()
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "surge")
	}
}

func GetStateDir() string {
	return filepath.Join(xdg.StateHome, "surge")
}

func GetDownloadsDir() string {
	return xdg.UserDirs.Download
}

func GetRuntimeDir() string {
	return filepath.Join(xdg.RuntimeDir, "surge")
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

// GetLogsDir returns the directory for logs
func GetLogsDir() string {
	return filepath.Join(GetStateDir(), "logs")
}

// EnsureDirs creates all required directories
func EnsureDirs() error {
	dirs := []string{GetSurgeDir(), GetStateDir(), GetRuntimeDir(), GetLogsDir()}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// On Linux runtime dir, we might want stricter permissions (0700) if it's in /run/user
	if runtime.GOOS == "linux" && os.Getenv("XDG_RUNTIME_DIR") != "" {
		_ = os.Chmod(GetRuntimeDir(), 0o700)
	}

	return nil
}
