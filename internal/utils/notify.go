package utils

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/gen2brain/beeep"
	"github.com/surge-downloader/surge/assets"
)

const NotificationAppName = "Surge"

var (
	iconPath string
	iconOnce sync.Once
)

func init() {
	beeep.AppName = NotificationAppName
}

// ensureIcon writes the notification icon to a user-private cache directory
// the first time it is called. Using sync.Once avoids the TOCTOU race that
// existed when the icon was written from init() to the shared os.TempDir().
func ensureIcon() string {
	iconOnce.Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			Debug("Failed to determine user cache dir: %v", err)
			return
		}
		surgeCache := filepath.Join(cacheDir, "surge")
		if err := os.MkdirAll(surgeCache, 0o755); err != nil {
			Debug("Failed to create icon cache dir: %v", err)
			return
		}
		iconPath = filepath.Join(surgeCache, "surge_logo.png")
		if err := os.WriteFile(iconPath, assets.LogoData, 0o644); err != nil {
			iconPath = ""
			Debug("Failed to write notification icon: %v", err)
		}
	})
	return iconPath
}

func Notify(title, message string) {
	err := beeep.Notify(title, message, ensureIcon())
	if err != nil {
		Debug("Failed to send notification: %v", err)
	}
}
