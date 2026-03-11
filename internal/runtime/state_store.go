package runtime

import (
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
)

type StateDownloadStore struct{}

var _ DownloadStore = StateDownloadStore{}

func NewDownloadStore() DownloadStore {
	return StateDownloadStore{}
}

func (StateDownloadStore) CheckExists(url string) (bool, error) {
	return state.CheckDownloadExists(url)
}

func (StateDownloadStore) HashURL(url string) string {
	return state.URLHash(url)
}

func (StateDownloadStore) Get(id string) (*types.DownloadEntry, error) {
	return state.GetDownload(id)
}

func (StateDownloadStore) Put(entry types.DownloadEntry) error {
	return state.AddToMasterList(entry)
}

func (StateDownloadStore) Remove(id string) error {
	return state.RemoveFromMasterList(id)
}

func (StateDownloadStore) LoadState(url string, destPath string) (*types.DownloadState, error) {
	return state.LoadState(url, destPath)
}

func (StateDownloadStore) SaveState(url string, destPath string, downloadState *types.DownloadState, skipFileHash bool) error {
	return state.SaveStateWithOptions(url, destPath, downloadState, state.SaveStateOptions{
		SkipFileHash: skipFileHash,
	})
}

func (StateDownloadStore) LoadStates(ids []string) (map[string]*types.DownloadState, error) {
	return state.LoadStates(ids)
}

func (StateDownloadStore) DeleteTasks(id string) error {
	return state.DeleteTasks(id)
}

func (StateDownloadStore) DeleteState(id string) error {
	return state.DeleteState(id)
}
