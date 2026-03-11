package processing

import (
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
)

type SettingsStore interface {
	Load() (*config.Settings, error)
	Save(*config.Settings) error
}

type DownloadStore interface {
	CheckExists(url string) (bool, error)
	HashURL(url string) string
	Get(id string) (*types.DownloadEntry, error)
	Put(entry types.DownloadEntry) error
	Remove(id string) error
	LoadState(url string, destPath string) (*types.DownloadState, error)
	SaveState(url string, destPath string, downloadState *types.DownloadState, skipFileHash bool) error
	LoadStates(ids []string) (map[string]*types.DownloadState, error)
	DeleteTasks(id string) error
	DeleteState(id string) error
}

type defaultSettingsStore struct{}

type defaultDownloadStore struct{}

func newDefaultSettingsStore() SettingsStore {
	return defaultSettingsStore{}
}

func newDefaultDownloadStore() DownloadStore {
	return defaultDownloadStore{}
}

func defaultSettings() *config.Settings {
	return config.DefaultSettings()
}

func (defaultSettingsStore) Load() (*config.Settings, error) {
	return config.LoadSettings()
}

func (defaultSettingsStore) Save(settings *config.Settings) error {
	return config.SaveSettings(settings)
}

func (defaultDownloadStore) CheckExists(url string) (bool, error) {
	return state.CheckDownloadExists(url)
}

func (defaultDownloadStore) HashURL(url string) string {
	return state.URLHash(url)
}

func (defaultDownloadStore) Get(id string) (*types.DownloadEntry, error) {
	return state.GetDownload(id)
}

func (defaultDownloadStore) Put(entry types.DownloadEntry) error {
	return state.AddToMasterList(entry)
}

func (defaultDownloadStore) Remove(id string) error {
	return state.RemoveFromMasterList(id)
}

func (defaultDownloadStore) LoadState(url string, destPath string) (*types.DownloadState, error) {
	return state.LoadState(url, destPath)
}

func (defaultDownloadStore) SaveState(url string, destPath string, downloadState *types.DownloadState, skipFileHash bool) error {
	return state.SaveStateWithOptions(url, destPath, downloadState, state.SaveStateOptions{
		SkipFileHash: skipFileHash,
	})
}

func (defaultDownloadStore) LoadStates(ids []string) (map[string]*types.DownloadState, error) {
	return state.LoadStates(ids)
}

func (defaultDownloadStore) DeleteTasks(id string) error {
	return state.DeleteTasks(id)
}

func (defaultDownloadStore) DeleteState(id string) error {
	return state.DeleteState(id)
}
