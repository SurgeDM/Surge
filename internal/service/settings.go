package service

import (
	"errors"
	"github.com/SurgeDM/Surge/internal/config"
)

var ErrInvalidSettings = errors.New("invalid settings")

// SettingsService is implemented by local and remote engines that own a
// persistent settings snapshot.
type SettingsService interface {
	GetSettings() (*config.Settings, error)
	UpdateSettings(*config.Settings) error
}

// SettingsEditor atomically merges edits against the snapshot they started from.
// It is separate from SettingsService so older settings providers stay readable.
type SettingsEditor interface {
	PatchSettings(config.SettingsPatch) (*config.Settings, error)
}
