package torrent

import (
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
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, float64(0), cfg.SeedRatio)
	assert.Equal(t, 50, cfg.MaxPeers)
}

func TestNewEngine_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()

	engine, err := NewEngine(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, engine)

	engine.Close()
}
