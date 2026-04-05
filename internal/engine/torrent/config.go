package torrent

import "time"

// Config holds torrent-specific settings.
type Config struct {
	// DataDir is the directory where torrent data is stored.
	DataDir string
	// SeedRatio is the upload-to-download ratio target. 0 means no seeding.
	SeedRatio float64
	// SeedTime is the maximum seeding duration after completion. 0 means no seeding.
	SeedTime time.Duration
	// MaxPeers is the max number of peers per torrent.
	MaxPeers int
	// ListenPort is the port to listen for incoming peer connections. 0 means random.
	ListenPort int
}

// DefaultConfig returns a Config with sensible defaults for a download-focused client.
func DefaultConfig() Config {
	return Config{
		DataDir:    "",
		SeedRatio:  0,
		SeedTime:   0,
		MaxPeers:   50,
		ListenPort: 0,
	}
}
