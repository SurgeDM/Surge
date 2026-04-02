package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/engine/concurrent"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/single"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/utils"
)

// safeSendProgress sends msg on ch, recovering from panics caused by sending
// on a closed channel (which can happen during shutdown).
func safeSendProgress(ch chan<- any, msg any) {
	defer func() { _ = recover() }()
	ch <- msg
}

// ProbeResult contains all metadata from server probe
type ProbeResult struct {
	FileSize      int64
	SupportsRange bool
	Filename      string
	ContentType   string
}

// uniqueFilePath returns a unique file path by appending (1), (2), etc. if the file exists
func uniqueFilePath(path string) string {
	// Check if file exists (both final and incomplete)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := os.Stat(path + types.IncompleteSuffix); os.IsNotExist(err) {
			return path // Neither exists, use original
		}
	}

	// File exists, generate unique name
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)

	// Check if name already has a counter like "file(1)"
	base := name
	counter := 1

	// Clean name to ensure parsing works even with trailing spaces
	cleanName := strings.TrimSpace(name)
	if len(cleanName) > 3 && cleanName[len(cleanName)-1] == ')' {
		if openParen := strings.LastIndexByte(cleanName, '('); openParen != -1 {
			// Try to parse number between parens
			numStr := cleanName[openParen+1 : len(cleanName)-1]
			if num, err := strconv.Atoi(numStr); err == nil && num > 0 {
				base = cleanName[:openParen]
				// Parsing "file (1)" -> "file " preserves original whitespace.
				counter = num + 1
			}
		}
	}

	for i := 0; i < 100; i++ { // Try next 100 numbers
		candidate := filepath.Join(dir, fmt.Sprintf("%s(%d)%s", base, counter+i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			if _, err := os.Stat(candidate + types.IncompleteSuffix); os.IsNotExist(err) {
				return candidate
			}
		}
	}

	// Fallback: just append a large random number or give up (original behavior essentially gave up or made ugly names)
	// Here we fallback to original behavior of appending if the clean one failed 100 times
	return path
}

// TUIDownload is the main entry point for downloads executed by the Engine pool
func TUIDownload(ctx context.Context, cfg *types.DownloadConfig) error {
	start := time.Now()
	// Engine expects cfg.OutputPath and cfg.Filename to be fully resolved by the processing layer
	destPath := cfg.OutputPath
	finalFilename := cfg.Filename
	finalDestPath := filepath.Join(destPath, finalFilename)

	// Local mirrors slice to avoid modifying config (race condition)
	mirrors := make([]string, len(cfg.Mirrors))
	copy(mirrors, cfg.Mirrors)

	// Check if this is a resume (explicitly marked by TUI)
	var savedState *types.DownloadState

	if cfg.IsResume && cfg.DestPath != "" {
		if cfg.SavedState != nil {
			savedState = cfg.SavedState
		}

		// Restore mirrors from state if found
		if savedState != nil && len(savedState.Mirrors) > 0 {
			// Create map of existing mirrors to avoid duplicates
			existing := make(map[string]bool)
			for _, m := range mirrors {
				existing[m] = true
			}

			// Add restored mirrors
			for _, m := range savedState.Mirrors {
				if !existing[m] {
					mirrors = append(mirrors, m)
					existing[m] = true
				}
			}
			utils.Debug("Restored %d mirrors from state", len(savedState.Mirrors))
		}
	}
	isResume := cfg.IsResume && savedState != nil && savedState.DestPath != ""

	if isResume {
		// Resume: use saved destination path directly (don't generate new unique name)
		finalDestPath = savedState.DestPath
		finalFilename = filepath.Base(finalDestPath)
		utils.Debug("Resuming download, using saved destPath: %s", finalDestPath)
	}
	utils.Debug("Destination path: %s", finalDestPath)

	if cfg.State != nil {
		cfg.State.SetFilename(finalFilename)
		cfg.State.SetDestPath(finalDestPath)
	}

	// Send download started message
	if cfg.ProgressCh != nil {
		safeSendProgress(cfg.ProgressCh, events.DownloadStartedMsg{
			DownloadID: cfg.ID,
			URL:        cfg.URL,
			Filename:   finalFilename,
			Total:      cfg.TotalSize, // Relies on TotalSize from Config
			DestPath:   finalDestPath,
			State:      cfg.State,
		})
	}

	// Update shared state
	if cfg.State != nil {
		cfg.State.SetTotalSize(cfg.TotalSize)
	}

	// Choose downloader based on probe results
	var downloadErr error
	if cfg.SupportsRange && cfg.TotalSize > 0 {
		utils.Debug("Using concurrent downloader")

		// We probe all candidate mirrors (mirrors) to filter out invalid ones
		var activeMirrors []string
		if len(mirrors) > 0 {
			utils.Debug("Probing %d mirrors", len(mirrors))
			// Always check primary + mirrors to ensure we are using the best set
			allToCheck := append([]string{cfg.URL}, mirrors...)
			valid, errs := processing.ProbeMirrorsWithProxy(ctx, allToCheck, cfg.Runtime.ProxyURL)

			// Log errors
			for u, e := range errs {
				utils.Debug("Mirror probe failed for %s: %v", u, e)
			}

			// Filter valid mirrors (excluding primary as it is handled separately)
			for _, v := range valid {
				if v != cfg.URL {
					activeMirrors = append(activeMirrors, v)
				}
			}
			utils.Debug("Found %d active mirrors from %d candidates", len(activeMirrors), len(mirrors))
		}

		d := concurrent.NewConcurrentDownloader(cfg.ID, cfg.ProgressCh, cfg.State, cfg.Runtime)
		d.Headers = cfg.Headers // Forward custom headers from browser extension
		utils.Debug("Calling Download with mirrors: %v", mirrors)
		downloadErr = d.Download(ctx, cfg.URL, mirrors, activeMirrors, finalDestPath, cfg.TotalSize)
	} else {
		// Fallback to single-threaded downloader
		utils.Debug("Using single-threaded downloader")
		d := single.NewSingleDownloader(cfg.ID, cfg.ProgressCh, cfg.State, cfg.Runtime)
		d.Headers = cfg.Headers // Forward custom headers from browser extension
		downloadErr = d.Download(ctx, cfg.URL, finalDestPath, cfg.TotalSize, finalFilename)
	}

	// Only send completion if NO error AND not paused
	// Check specifically for ErrPaused to avoid treating it as error
	if errors.Is(downloadErr, types.ErrPaused) {
		utils.Debug("Download paused cleanly")
		return nil // Return nil so worker can remove it from active map
	}

	isPaused := cfg.State != nil && cfg.State.IsPaused()
	if downloadErr == nil && !isPaused {
		var elapsed time.Duration
		if cfg.State != nil {
			_, elapsed = cfg.State.FinalizeSession(cfg.TotalSize)
		} else {
			elapsed = time.Since(start)
		}

		// Persist to history before sending event
		// Compute average download speed in bytes/sec
		var avgSpeed float64
		if elapsed.Seconds() > 0 {
			avgSpeed = float64(cfg.TotalSize) / elapsed.Seconds()
		}

		if cfg.ProgressCh != nil {
			safeSendProgress(cfg.ProgressCh, events.DownloadCompleteMsg{
				DownloadID: cfg.ID,
				Filename:   finalFilename,
				Elapsed:    elapsed,
				Total:      cfg.TotalSize,
				AvgSpeed:   avgSpeed,
			})
		}
	} else if downloadErr != nil && !isPaused {
		// Verify it's not a cancellation error
		if errors.Is(downloadErr, context.Canceled) {
			utils.Debug("Download canceled cleanly")
			return nil
		}

		// Send error event
		if cfg.ProgressCh != nil {
			safeSendProgress(cfg.ProgressCh, events.DownloadErrorMsg{
				DownloadID: cfg.ID,
				Filename:   finalFilename,
				DestPath:   finalDestPath,
				Err:        downloadErr,
			})
		}
	}

	return downloadErr
}

// Download is the CLI entry point (non-TUI) - convenience wrapper
func Download(ctx context.Context, url string, outPath string, progressCh chan<- any, id string) error {
	cfg := types.DownloadConfig{
		URL:        url,
		OutputPath: outPath,
		ID:         id,
		ProgressCh: progressCh,
		State:      nil,
	}
	// Default runtime config
	cfg.Runtime = &types.RuntimeConfig{
		MaxConnectionsPerHost: types.PerHostMax,
		MinChunkSize:          types.MinChunk,
		WorkerBufferSize:      types.WorkerBuffer,
	}
	return TUIDownload(ctx, &cfg)
}
