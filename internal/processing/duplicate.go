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

// CheckForDuplicate inspects the active downloads to see if a compatible download already exists
func CheckForDuplicate(url string, settings *config.Settings, activeDownloads func() map[string]*types.DownloadConfig) *DuplicateResult {
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

	return nil
}
