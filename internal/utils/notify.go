package utils

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/SurgeDM/Surge/assets"
	"github.com/gen2brain/beeep"
)

const NotificationAppName = "Surge"

// SuppressNotifications can be set to true to prevent desktop notifications.
// Tests should set this to true via TestMain or init() to avoid notification spam.
var SuppressNotifications bool

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
			Debug("Failed to determine user cache dir, falling back to temp: %v", err)
			cacheDir = os.TempDir()
		}
		surgeCache := filepath.Join(cacheDir, "surge")
		if err := os.MkdirAll(surgeCache, 0o755); err != nil {
			Debug("Failed to create icon cache dir: %v", err)
			return
		}
		path := filepath.Join(surgeCache, "surge_logo.png")
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			if err := os.WriteFile(path, assets.LogoData, 0o644); err != nil {
				Debug("Failed to write notification icon: %v", err)
				return
			}
		}
		iconPath = path
	})
	return iconPath
}

func Notify(title, message string) {
	if SuppressNotifications {
		return
	}
	err := beeep.Notify(title, message, ensureIcon())
	if err != nil {
		Debug("Failed to send notification: %v", err)
	}
}
