package surge

import (
	"context"

	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
)

// NewWorkerPool creates a download worker pool.
func NewWorkerPool(progressCh chan<- any, maxDownloads int) *WorkerPool {
	return download.NewWorkerPool(progressCh, maxDownloads)
}

// NewLocalDownloadService creates an embedded download service backed by pool.
func NewLocalDownloadService(pool *WorkerPool) *LocalDownloadService {
	return core.NewLocalDownloadService(pool)
}

// NewLocalDownloadServiceWithInput creates an embedded service with a caller-provided event channel.
func NewLocalDownloadServiceWithInput(pool *WorkerPool, inputCh chan any) *LocalDownloadService {
	return core.NewLocalDownloadServiceWithInput(pool, inputCh)
}

// NewRemoteDownloadService creates a client for a remote Surge daemon.
func NewRemoteDownloadService(baseURL string, token string, opts HTTPClientOptions) (*RemoteDownloadService, error) {
	return core.NewRemoteDownloadService(baseURL, token, opts)
}

// ConfigureStateDB configures the SQLite state database used for history and resume metadata.
func ConfigureStateDB(path string) {
	state.Configure(path)
}

// CloseStateDB closes and resets the configured state database.
func CloseStateDB() {
	state.CloseDB()
}

// Download runs a single download without constructing a service.
func Download(ctx context.Context, url string, outPath string, progressCh chan<- any, id string) error {
	return download.Download(ctx, url, outPath, progressCh, id)
}
