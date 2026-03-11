package runtime

import "github.com/surge-downloader/surge/internal/config"

type FileSettingsStore struct{}

var _ SettingsStore = FileSettingsStore{}

func NewSettingsStore() SettingsStore {
	return FileSettingsStore{}
}

func (FileSettingsStore) Load() (*config.Settings, error) {
	return config.LoadSettings()
}

func (FileSettingsStore) Save(settings *config.Settings) error {
	return config.SaveSettings(settings)
}
