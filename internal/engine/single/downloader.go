package single

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/engine"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// SingleDownloader handles single-threaded downloads for servers that don't support range requests.
// NOTE: Pause/resume is NOT supported because this downloader is only used when
// the server doesn't support Range headers. If interrupted, the download must restart.
type SingleDownloader struct {
	ProgressChan chan<- any           // Channel for events (start/complete/error)
	ID           string               // Download ID
	State        *types.ProgressState // Shared state for TUI polling
	Runtime      *types.RuntimeConfig
	TotalSize    int64
	Headers      map[string]string // Custom HTTP headers (cookies, auth, etc.)
}

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*types.KB)
		return &b
	},
}

// NewSingleDownloader creates a new single-threaded downloader with all required parameters
func NewSingleDownloader(id string, progressCh chan<- any, state *types.ProgressState, runtime *types.RuntimeConfig) *SingleDownloader {
	if runtime == nil {
		runtime = &types.RuntimeConfig{}
	}

	return &SingleDownloader{
		ProgressChan: progressCh,
		ID:           id,
		State:        state,
		Runtime:      runtime,
	}
}

func (d *SingleDownloader) applyClientSettings(client *http.Client) {
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if len(via) > 0 {
			utils.CopyRedirectHeaders(req, via[0])
		}
		if d.Headers != nil {
			for key, val := range d.Headers {
				if key != "Range" {
					req.Header.Set(key, val)
				}
			}
		}
		return nil
	}
}

// Download downloads a file using a single connection.
// This is used for servers that don't support Range requests.
// If interrupted, the download cannot be resumed and must restart from the beginning.
func (d *SingleDownloader) Download(ctx context.Context, rawurl, destPath string, fileSize int64, filename string) (err error) {
	transport := engine.DefaultNetworkPool.AcquireTransport(d.Runtime.ProxyURL, d.Runtime.CustomDNS, types.PoolMaxConnsPerHost)
	defer engine.DefaultNetworkPool.ReleaseTransport(transport)

	client := &http.Client{Transport: transport}
	d.applyClientSettings(client)

	if d.State != nil {
		d.State.SetURL(rawurl)
		d.State.SetDestPath(destPath)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return err
	}

	for key, val := range d.Headers {
		req.Header.Set(key, val)
	}
	req.Header.Set("User-Agent", d.Runtime.GetUserAgent())

	resp, err := client.Do(req)
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

	if fileSize <= 0 && resp.ContentLength > 0 {
		fileSize = resp.ContentLength
	}
	d.TotalSize = fileSize
	if d.State != nil && fileSize > 0 {
		d.State.SetTotalSize(fileSize)
	}

	// Use .surge extension for incomplete file (must be pre-created by processing layer)
	workingPath := destPath + types.IncompleteSuffix
	outFile, err := os.OpenFile(workingPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := outFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close error: %w", cerr)
		}
	}()

	preallocated := false
	if fileSize > 0 {
		if err := preallocateFile(outFile, fileSize); err != nil {
			return fmt.Errorf("failed to preallocate file: %w", err)
		}
		preallocated = true
	}

	start := time.Now()
	var written int64

	bufPtr := bufPool.Get().(*[]byte)
	buf := *bufPtr
	defer bufPool.Put(bufPtr)

	if d.State == nil {
		written, err = io.CopyBuffer(outFile, resp.Body, buf)
	} else {
		progressReader := newProgressReader(resp.Body, d.State, types.WorkerBatchSize, types.WorkerBatchInterval)
		written, err = io.CopyBuffer(outFile, progressReader, buf)
		progressReader.Flush()
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

	if d.State != nil {
		d.State.Downloaded.Store(written)
		d.State.VerifiedProgress.Store(written)
	}

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

type progressReader struct {
	reader        io.Reader
	state         *types.ProgressState
	batchSize     int64
	batchInterval time.Duration
	written       int64
	pending       int64
	lastFlush     time.Time
	readChecks    uint8
}

func newProgressReader(reader io.Reader, state *types.ProgressState, batchSize int64, batchInterval time.Duration) *progressReader {
	if batchSize <= 0 {
		batchSize = types.WorkerBatchSize
	}
	return &progressReader{
		reader:        reader,
		state:         state,
		batchSize:     batchSize,
		batchInterval: batchInterval,
		lastFlush:     time.Now(),
	}
}

func (w *progressReader) Read(p []byte) (int, error) {
	n, err := w.reader.Read(p)
	if n <= 0 || w.state == nil {
		return n, err
	}

	written := int64(n)
	w.written += written
	w.pending += written
	if w.pending >= w.batchSize {
		w.flushWithTime(time.Now())
		return n, err
	}

	if w.batchInterval > 0 {
		// Check wall-clock interval periodically to avoid calling time.Now on every read.
		w.readChecks++
		if w.readChecks >= 8 {
			now := time.Now()
			if now.Sub(w.lastFlush) >= w.batchInterval {
				w.flushWithTime(now)
			}
			w.readChecks = 0
		}
	}

	return n, err
}

func (w *progressReader) Flush() {
	w.flushWithTime(time.Now())
}

func (w *progressReader) flushWithTime(now time.Time) {
	if w.state == nil {
		w.pending = 0
		w.lastFlush = now
		w.readChecks = 0
		return
	}

	if w.pending == 0 && w.written == 0 {
		return
	}

	w.state.Downloaded.Store(w.written)
	w.state.VerifiedProgress.Store(w.written)
	w.pending = 0
	w.lastFlush = now
	w.readChecks = 0
}
