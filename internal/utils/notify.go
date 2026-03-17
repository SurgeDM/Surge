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

	iconPath = filepath.Join(os.TempDir(), "surge_logo.png")

	err := os.WriteFile(iconPath, assets.LogoData, 0644)
	if err != nil {
		iconPath = ""
		Debug("Failed to write notification icon: %v", err)
	}
}

func Notify(message string) {

	err := beeep.Notify(NotificationAppName, message, iconPath)
	if err != nil {
		Debug("Failed to send notification: %v", err)
	}
}
