package torrent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

// Engine wraps anacrolix/torrent.Client and adapts it for Surge.
type Engine struct {
	client *torrent.Client
	config Config
}

// Progress represents the current state of a torrent download.
type Progress struct {
	BytesCompleted int64
	BytesTotal     int64
	PeerCount      int
	SeedCount      int
	Speed          float64 // bytes/sec (caller computes from deltas)
	Name           string
	Done           bool
}

// NewEngine creates and starts a new torrent engine.
func NewEngine(cfg Config) (*Engine, error) {
	clientCfg := torrent.NewDefaultClientConfig()

	if cfg.DataDir != "" {
		clientCfg.DefaultStorage = storage.NewFileByInfoHash(cfg.DataDir)
		clientCfg.DataDir = cfg.DataDir
	}

	if cfg.ListenPort > 0 {
		clientCfg.ListenPort = cfg.ListenPort
	}

	// Disable upload by default (seed ratio 0)
	if cfg.SeedRatio <= 0 && cfg.SeedTime <= 0 {
		clientCfg.NoUpload = true
		clientCfg.Seed = false
	} else {
		clientCfg.Seed = true
	}

	client, err := torrent.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("torrent: failed to create client: %w", err)
	}

	return &Engine{
		client: client,
		config: cfg,
	}, nil
}

// Close shuts down the torrent engine gracefully.
func (e *Engine) Close() {
	if e.client != nil {
		e.client.Close()
	}
}

// AddMagnet adds a magnet link and returns a handle to monitor progress.
func (e *Engine) AddMagnet(magnetURI string) (*torrent.Torrent, error) {
	t, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("torrent: failed to add magnet: %w", err)
	}
	return t, nil
}

// AddTorrentFile adds a .torrent file and returns a handle.
func (e *Engine) AddTorrentFile(path string) (*torrent.Torrent, error) {
	t, err := e.client.AddTorrentFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("torrent: failed to add torrent file: %w", err)
	}
	return t, nil
}

// Download starts or resumes downloading a torrent and blocks until complete or
// the context is cancelled. It reports progress to the provided channel.
func (e *Engine) Download(ctx context.Context, t *torrent.Torrent, progressCh chan<- Progress) error {
	// Wait for torrent info (metadata) with timeout
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("torrent: timed out waiting for metadata")
	}

	// Start downloading all files
	t.DownloadAll()

	// Report progress periodically
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastCompleted int64
	lastTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info := t.Info()
			if info == nil {
				continue
			}

			completed := t.BytesCompleted()
			total := info.TotalLength()
			now := time.Now()

			elapsed := now.Sub(lastTime).Seconds()
			var speed float64
			if elapsed > 0 {
				speed = float64(completed-lastCompleted) / elapsed
			}
			lastCompleted = completed
			lastTime = now

			stats := t.Stats()
			done := completed >= total

			if progressCh != nil {
				progressCh <- Progress{
					BytesCompleted: completed,
					BytesTotal:     total,
					PeerCount:      stats.ActivePeers,
					SeedCount:      stats.ConnectedSeeders,
					Speed:          speed,
					Name:           t.Name(),
					Done:           done,
				}
			}

			if done {
				return nil
			}
		}
	}
}

// IsMagnetURI checks if a string looks like a magnet link.
func IsMagnetURI(s string) bool {
	return strings.HasPrefix(s, "magnet:?")
}

// IsTorrentFile checks if a path points to a .torrent file.
func IsTorrentFile(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(filepath.Ext(path)), ".torrent")
}
