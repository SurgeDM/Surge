package torrent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsMagnetURI(t *testing.T) {
	assert.True(t, IsMagnetURI("magnet:?xt=urn:btih:abc123"))
	assert.True(t, IsMagnetURI("magnet:?xt=urn:btih:KRWPCX3SJUM4IMM4YF5RPHL6ANPYTQPU"))
	assert.False(t, IsMagnetURI("https://example.com/file.zip"))
	assert.False(t, IsMagnetURI(""))
	assert.False(t, IsMagnetURI("magnet"))
}

func TestIsTorrentFile(t *testing.T) {
	assert.False(t, IsTorrentFile("/nonexistent/file.torrent"))
	assert.False(t, IsTorrentFile("/tmp/test.txt"))

	// Positive case: real .torrent file
	f, err := os.CreateTemp(t.TempDir(), "*.torrent")
	assert.NoError(t, err)
	f.Close()
	assert.True(t, IsTorrentFile(f.Name()))

	// Should NOT match extensions that merely end in "torrent"
	f2, err := os.CreateTemp(t.TempDir(), "*.mytorrent")
	assert.NoError(t, err)
	f2.Close()
	assert.False(t, IsTorrentFile(f2.Name()))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, float64(0), cfg.SeedRatio)
	assert.Equal(t, 50, cfg.MaxPeers)
}

func TestNewEngine_DefaultConfig(t *testing.T) {
	// Use a manually managed temp dir instead of t.TempDir() because the
	// anacrolix/torrent SQLite storage may hold file locks briefly after
	// Close() returns on Windows, causing TempDir cleanup to fail.
	dir, err := os.MkdirTemp("", "surge-torrent-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	cfg := DefaultConfig()
	cfg.DataDir = dir

	engine, err := NewEngine(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, engine)

	engine.Close()
}
