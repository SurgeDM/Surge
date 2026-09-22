package service

import "github.com/SurgeDM/Surge/internal/config"

// SettingsService is implemented by local and remote engines that own a
// persistent settings snapshot.
type SettingsService interface {
	GetSettings() (*config.Settings, error)
	UpdateSettings(*config.Settings) error
}
