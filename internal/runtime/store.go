package runtime

import (
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
)

// SettingsStore abstracts persisted settings access behind the runtime layer.
type SettingsStore interface {
	Load() (*config.Settings, error)
	Save(*config.Settings) error
}

// DownloadStore abstracts persisted download metadata and checkpoints behind the runtime layer.
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
