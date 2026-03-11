package runtime

import "github.com/surge-downloader/surge/internal/config"

// LoadSettingsOrDefault returns persisted settings or defaults when loading fails.
func LoadSettingsOrDefault() *config.Settings {
	settings, err := config.LoadSettings()
	if err != nil || settings == nil {
		return config.DefaultSettings()
	}
	return settings
}

func (a *App) Settings() *config.Settings {
	if a.settings != nil {
		return a.settings
	}

	a.settings = LoadSettingsOrDefault()
	return a.settings
}

func (a *App) ApplySettings(settings *config.Settings) {
	if settings == nil {
		settings = config.DefaultSettings()
	}
	a.settings = settings
}

func (a *App) ReloadSettings() *config.Settings {
	a.settings = LoadSettingsOrDefault()
	return a.settings
}
