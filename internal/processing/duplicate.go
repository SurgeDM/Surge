package processing

import (
	"strings"

	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

// DuplicateResult represents the outcome of a duplicate check
type DuplicateResult struct {
	Exists   bool
	IsActive bool
	Filename string
	URL      string
}

// CheckForDuplicate inspects active and persisted downloads for duplicate URLs.
// It always performs the scan regardless of settings.General.WarnOnDuplicate.
// Policy decisions (whether to warn, block, or auto-approve) are the caller's
// responsibility. This separation is required so that headless mode can always
// distinguish duplicates from new downloads, even when WarnOnDuplicate is off.
func CheckForDuplicate(url string, activeDownloads func() map[string]*types.DownloadConfig) *DuplicateResult {
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
	if exists, err := state.CheckDownloadExists(normalizedInputURL); err == nil && exists {
		return &DuplicateResult{
			Exists:   true,
			IsActive: false,
			URL:      normalizedInputURL,
		}
	}

	return nil
}
