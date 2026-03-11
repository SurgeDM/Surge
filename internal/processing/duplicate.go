package processing

import (
	"strings"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
)

// DuplicateResult represents the outcome of a duplicate check
type DuplicateResult struct {
	Exists   bool
	IsActive bool
	Filename string
	URL      string
}

// CheckForDuplicate inspects active and persisted downloads for duplicate URLs.
func CheckForDuplicate(url string, settings *config.Settings, activeDownloads func() map[string]*types.DownloadConfig) *DuplicateResult {
	return checkForDuplicate(url, settings, activeDownloads, newDefaultDownloadStore())
}

func checkForDuplicate(url string, settings *config.Settings, activeDownloads func() map[string]*types.DownloadConfig, store DownloadStore) *DuplicateResult {
	if !settings.General.WarnOnDuplicate {
		return nil
	}

	normalizedInputURL := strings.TrimRight(url, "/")

	// Check active downloads
	if activeDownloads != nil {
		active := activeDownloads()
		for _, d := range active {
			normalizedExistingURL := strings.TrimRight(d.URL, "/")
			if normalizedExistingURL == normalizedInputURL {
				isActive := false
				if d.State != nil && !d.State.Done.Load() {
					isActive = true
				}

				return &DuplicateResult{
					Exists:   true,
					IsActive: isActive,
					Filename: d.Filename,
					URL:      d.URL,
				}
			}
		}
	}

	// Check persisted completed/paused/queued entries in DB.
	if exists, err := store.CheckExists(normalizedInputURL); err == nil && exists {
		return &DuplicateResult{
			Exists:   true,
			IsActive: false,
			URL:      normalizedInputURL,
		}
	}

	return nil
}
