package processing

import (
	"github.com/surge-downloader/surge/internal/config"
)

// LifecycleManager orchestrates the life of a download outside of the core HTTP engine.
// It handles probing, category routing, file conflict resolution, and settings management.
type LifecycleManager struct {
	settings *config.Settings
}

func NewLifecycleManager() *LifecycleManager {
	// 1. Load Settings immediately on startup as part of the processing layer's responsibility
	settings, err := config.LoadSettings()
	if err != nil {
		// If settings fail to load, fallback to defaults
		settings = config.DefaultSettings()
	}

	return &LifecycleManager{
		settings: settings,
	}
}

// GetSettings allows the UI to read the current settings (e.g. for the Settings view).
func (m *LifecycleManager) GetSettings() *config.Settings {
	return m.settings
}

// SaveSettings persists settings and updates the active instance.
func (m *LifecycleManager) SaveSettings(s *config.Settings) error {
	if err := config.SaveSettings(s); err != nil {
		return err
	}
	m.settings = s
	return nil
}
