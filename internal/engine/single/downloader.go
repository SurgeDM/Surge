package single

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// SingleDownloader handles single-threaded downloads for servers that don't support range requests.
// NOTE: Pause/resume is NOT supported because this downloader is only used when
// the server doesn't support Range headers. If interrupted, the download must restart.
type SingleDownloader struct {
	Client       *http.Client
	ProgressChan chan<- any           // Channel for events (start/complete/error)
	ID           string               // Download ID
	State        *types.ProgressState // Shared state for TUI polling
	Runtime      *types.RuntimeConfig
	Headers      map[string]string // Custom HTTP headers (cookies, auth, etc.)
}

// NewSingleDownloader creates a new single-threaded downloader with all required parameters
func NewSingleDownloader(id string, progressCh chan<- any, state *types.ProgressState, runtime *types.RuntimeConfig) *SingleDownloader {
	return &SingleDownloader{
		Client:       &http.Client{Timeout: 0},
		ProgressChan: progressCh,
		ID:           id,
		State:        state,
		Runtime:      runtime,
	}
}

// Download downloads a file using a single connection.
// This is used for servers that don't support Range requests.
// If interrupted, the download cannot be resumed and must restart from the beginning.
func (d *SingleDownloader) Download(ctx context.Context, rawurl, destPath string, fileSize int64, filename string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return err
	}

	for key, val := range d.Headers {
		req.Header.Set(key, val)
	}
	req.Header.Set("User-Agent", d.Runtime.GetUserAgent())

	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			utils.Debug("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Use .surge extension for incomplete file
	workingPath := destPath + types.IncompleteSuffix
	outFile, err := os.Create(workingPath)
	if err != nil {
		return err
	}

	preallocated := false
	if fileSize > 0 {
		if err := outFile.Truncate(fileSize); err != nil {
			return fmt.Errorf("failed to preallocate file: %w", err)
		}
		if _, err := outFile.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to reset file offset: %w", err)
		}
		preallocated = true
	}

	// Track whether we completed successfully for cleanup
	success := false
	defer func() {
		_ = outFile.Close()
		if !success {
			_ = os.Remove(workingPath)
		}
	}()

	start := time.Now()
	buf := make([]byte, d.Runtime.GetWorkerBufferSize())
	var written int64

	if d.State == nil {
		written, err = io.CopyBuffer(outFile, resp.Body, buf)
	} else {
		progressWriter := newProgressWriter(outFile, d.State, types.WorkerBatchSize, types.WorkerBatchInterval)
		written, err = io.CopyBuffer(progressWriter, resp.Body, buf)
		progressWriter.Flush()
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("copy error: %w", err)
	}

	if preallocated && written != fileSize {
		if err := outFile.Truncate(written); err != nil {
			return fmt.Errorf("truncate error: %w", err)
		}
	}

	if err := outFile.Sync(); err != nil {
		return fmt.Errorf("sync error: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("close error: %w", err)
	}

	// Rename .surge file to final destination
	if err := os.Rename(workingPath, destPath); err != nil {
		// Fallback: copy if rename fails (cross-device)
		if copyErr := copyFile(workingPath, destPath); copyErr != nil {
			return fmt.Errorf("failed to finalize file: %w", copyErr)
		}
		_ = os.Remove(workingPath)
	}

	success = true // Mark successful so defer doesn't clean up

	elapsed := time.Since(start)
	speed := 0.0
	if elapsed > 0 {
		speed = float64(written) / elapsed.Seconds()
	}
	utils.Debug("\nDownloaded %s in %s (%s/s)\n",
		destPath,
		elapsed.Round(time.Second),
		utils.ConvertBytesToHumanReadable(int64(speed)),
	)

	return nil
}

type progressWriter struct {
	file          *os.File
	state         *types.ProgressState
	batchSize     int64
	batchInterval time.Duration
	written       int64
	pending       int64
	lastFlush     time.Time
}

func newProgressWriter(file *os.File, state *types.ProgressState, batchSize int64, batchInterval time.Duration) *progressWriter {
	if batchSize <= 0 {
		batchSize = types.WorkerBatchSize
	}
	return &progressWriter{
		file:          file,
		state:         state,
		batchSize:     batchSize,
		batchInterval: batchInterval,
		lastFlush:     time.Now(),
	}
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if n <= 0 || w.state == nil {
		return n, err
	}

	written := int64(n)
	w.written += written
	w.pending += written
	if w.pending >= w.batchSize || (w.batchInterval > 0 && time.Since(w.lastFlush) >= w.batchInterval) {
		w.Flush()
	}
	return n, err
}

func (w *progressWriter) Flush() {
	if w.state == nil {
		w.pending = 0
		w.lastFlush = time.Now()
		return
	}

	if w.pending == 0 && w.written == 0 {
		return
	}

	w.state.Downloaded.Store(w.written)
	w.state.VerifiedProgress.Store(w.written)
	w.pending = 0
	w.lastFlush = time.Now()
}

// copyFile copies a file from src to dst (fallback when rename fails)
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			utils.Debug("Error closing input file: %v", err)
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			utils.Debug("Error closing output file: %v", err)
		}
	}()

	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(out, in, buf); err != nil {
		return err
	}
	return out.Sync()
}
