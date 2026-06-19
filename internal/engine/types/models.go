package types

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
)

// Task represents a byte range to download.
type Task struct {
	Offset          int64         `json:"offset"`
	Length          int64         `json:"length"`
	SharedMaxOffset *atomic.Int64 `json:"-"`
}

func (t *Task) GobEncode() ([]byte, error) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], uint64(t.Offset))
	binary.LittleEndian.PutUint64(b[8:16], uint64(t.Length))
	return b, nil
}

func (t *Task) GobDecode(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if len(data) < 16 {
		return fmt.Errorf("corrupt task data: expected 16 bytes, got %d", len(data))
	}
	t.Offset = int64(binary.LittleEndian.Uint64(data[0:8]))
	t.Length = int64(binary.LittleEndian.Uint64(data[8:16]))
	return nil
}

// DownloadState is the persisted snapshot used to resume a download.
type DownloadState struct {
	ID         string   `json:"id"`
	URLHash    string   `json:"url_hash"`
	URL        string   `json:"url"`
	DestPath   string   `json:"dest_path"`
	TotalSize  int64    `json:"total_size"`
	Downloaded int64    `json:"downloaded"`
	Tasks      []Task   `json:"tasks"`
	Filename   string   `json:"filename"`
	CreatedAt  int64    `json:"created_at"`
	PausedAt   int64    `json:"paused_at"`
	Elapsed    int64    `json:"elapsed"`
	Mirrors    []string `json:"mirrors,omitempty"`

	ChunkBitmap     []byte `json:"chunk_bitmap,omitempty"`
	ActualChunkSize int64  `json:"actual_chunk_size,omitempty"`

	FileHash     string `json:"file_hash,omitempty"`
	RateLimit    int64  `json:"rate_limit,omitempty"`
	RateLimitSet bool   `json:"rate_limit_set,omitempty"`
}

// DownloadEntry is the durable record used for history and lifecycle recovery.
type DownloadEntry struct {
	ID           string   `json:"id"`
	URLHash      string   `json:"url_hash"`
	URL          string   `json:"url"`
	DestPath     string   `json:"dest_path"`
	Filename     string   `json:"filename"`
	Status       string   `json:"status"`
	TotalSize    int64    `json:"total_size"`
	Downloaded   int64    `json:"downloaded"`
	CompletedAt  int64    `json:"completed_at"`
	TimeTaken    int64    `json:"time_taken"`
	AvgSpeed     float64  `json:"avg_speed"`
	Mirrors      []string `json:"mirrors,omitempty"`
	RateLimit    int64    `json:"rate_limit,omitempty"`
	RateLimitSet bool     `json:"rate_limit_set,omitempty"`
}

// MasterList holds all tracked downloads.
type MasterList struct {
	Downloads []DownloadEntry `json:"downloads"`
}

// DownloadStatus is the transient view returned to the TUI and API clients.
type DownloadStatus struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Filename     string  `json:"filename"`
	DestPath     string  `json:"dest_path,omitempty"`
	TotalSize    int64   `json:"total_size"`
	Downloaded   int64   `json:"downloaded"`
	Progress     float64 `json:"progress"`
	Speed        float64 `json:"speed"`
	Status       string  `json:"status"`
	Error        string  `json:"error,omitempty"`
	ETA          int64   `json:"eta"`
	Connections  int     `json:"connections"`
	AddedAt      int64   `json:"added_at"`
	TimeTaken    int64   `json:"time_taken"`
	AvgSpeed     float64 `json:"avg_speed"`
	RateLimit    int64   `json:"rate_limit,omitempty"`
	RateLimitSet bool    `json:"rate_limit_set,omitempty"`
}

// CancelResult carries enough metadata for callers to emit lifecycle events
// without creating an import cycle back to the worker pool.
type CancelResult struct {
	Found     bool
	Filename  string
	DestPath  string
	Completed bool
	WasQueued bool
}
