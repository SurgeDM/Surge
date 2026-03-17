package utils

import (
	"os"
	"path/filepath"

	"github.com/gen2brain/beeep"
	"github.com/surge-downloader/surge/assets"
)

const NotificationAppName = "Surge"

var iconPath string

func init() {
	beeep.AppName = NotificationAppName

	iconPath = filepath.Join(os.TempDir(), "surge_logo.png")

	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(iconPath, assets.LogoData, 0644); writeErr != nil {
			iconPath = ""
			Debug("Failed to write notification icon: %v", writeErr)
		}
	}
}

func Notify(title, message string) {

	err := beeep.Notify(title, message, iconPath)
	if err != nil {
		Debug("Failed to send notification: %v", err)
	}
}
