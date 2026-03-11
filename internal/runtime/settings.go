package runtime

import "github.com/surge-downloader/surge/internal/config"

func (a *App) Settings() *config.Settings {
	if a.settings != nil {
		return a.settings
	}

	settings, err := config.LoadSettings()
	if err != nil || settings == nil {
		settings = config.DefaultSettings()
	}
	a.settings = settings
	return settings
}

func (a *App) ApplySettings(settings *config.Settings) {
	if settings == nil {
		settings = config.DefaultSettings()
	}
	a.settings = settings
}

func (a *App) ReloadSettings() *config.Settings {
	settings, err := config.LoadSettings()
	if err != nil || settings == nil {
		settings = config.DefaultSettings()
	}
	a.settings = settings
	return settings
}
