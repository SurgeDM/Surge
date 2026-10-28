package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

// ResultService supplies completion after the lifecycle worker has finalized the
// artifact and committed its record. GetStatus remains a transient UI snapshot.
// Keep this optional so older and third-party DownloadService implementations
// cannot accidentally claim the stronger completion contract.
type ResultService interface {
	GetResult(id string) (*types.DownloadStatus, error)
}

var ErrArtifactUnavailable = errors.New("completed artifact is missing, inaccessible, or has changed size")

func (s *LocalDownloadService) GetResult(id string) (*types.DownloadStatus, error) {
	var live *types.DownloadStatus
	if s.lifecycle != nil && s.lifecycle.GetScheduler() != nil {
		live = s.lifecycle.GetScheduler().GetStatus(id)
	}
	if live != nil && (live.Status == "downloading" || live.Status == "queued" || live.Status == "pausing" || live.Status == "verifying") {
		return live, nil
	}
	entry, err := store.GetDownload(id)
	if err != nil && !errors.Is(err, types.ErrNotFound) {
		return nil, err
	}
	// In-memory terminal events can precede persistence. Completed/error
	// records come from the serialized lifecycle worker; a paused record only
	// confirms a pause when the current worker is also paused.
	committed := entry != nil && (entry.Status == "completed" || entry.Status == "error")
	if entry != nil && live != nil && entry.Status == "paused" && live.Status == "paused" {
		committed = true
	}
	if live != nil && !committed {
		status := *live
		status.Speed = 0
		if status.Status == "paused" {
			status.Status = "pausing"
		} else {
			status.Status = "finalizing"
		}
		return &status, nil
	}
	if entry == nil {
		return nil, types.ErrNotFound
	}
	status := statusFromDownloadRecord(*entry)
	if status.Status == "completed" {
		if entry.DestPath == "" {
			return nil, ErrArtifactUnavailable
		}
		path, err := filepath.Abs(entry.DestPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrArtifactUnavailable, err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.Downloaded {
			return nil, ErrArtifactUnavailable
		}
		status.DestPath = path
	}
	return &status, nil
}
