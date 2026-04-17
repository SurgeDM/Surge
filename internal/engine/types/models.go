package types

import "sync/atomic"

// Task represents a byte range to download
type Task struct {
	SharedMaxOffset *atomic.Int64 `json:"-"`
	Offset          int64         `json:"offset"`
	Length          int64         `json:"length"`
}

// DownloadState represents persisted download state for resume
type DownloadState struct {
	ID              string   `json:"id"`
	URLHash         string   `json:"url_hash"`
	URL             string   `json:"url"`
	DestPath        string   `json:"dest_path"`
	FileHash        string   `json:"file_hash,omitempty"`
	Filename        string   `json:"filename"`
	Tasks           []Task   `json:"tasks"`
	Mirrors         []string `json:"mirrors,omitempty"`
	ChunkBitmap     []byte   `json:"chunk_bitmap,omitempty"`
	Downloaded      int64    `json:"downloaded"`
	CreatedAt       int64    `json:"created_at"`
	PausedAt        int64    `json:"paused_at"`
	Elapsed         int64    `json:"elapsed"`
	ActualChunkSize int64    `json:"actual_chunk_size,omitempty"`
	TotalSize       int64    `json:"total_size"`
}

// DownloadEntry represents a download in the master list
type DownloadEntry struct {
	ID          string   `json:"id"`
	URLHash     string   `json:"url_hash"`
	URL         string   `json:"url"`
	DestPath    string   `json:"dest_path"`
	Filename    string   `json:"filename"`
	Status      string   `json:"status"`
	Mirrors     []string `json:"mirrors,omitempty"`
	TotalSize   int64    `json:"total_size"`
	Downloaded  int64    `json:"downloaded"`
	CompletedAt int64    `json:"completed_at"`
	TimeTaken   int64    `json:"time_taken"`
	AvgSpeed    float64  `json:"avg_speed"`
}

// MasterList holds all tracked downloads
type MasterList struct {
	Downloads []DownloadEntry `json:"downloads"`
}

// DownloadStatus represents the transient status of an active download
type DownloadStatus struct {
	Error       string  `json:"error,omitempty"`
	URL         string  `json:"url"`
	Filename    string  `json:"filename"`
	DestPath    string  `json:"dest_path,omitempty"`
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	ETA         int64   `json:"eta"`
	Downloaded  int64   `json:"downloaded"`
	Progress    float64 `json:"progress"`
	TotalSize   int64   `json:"total_size"`
	Speed       float64 `json:"speed"`
	Connections int     `json:"connections"`
	AddedAt     int64   `json:"added_at"`
	TimeTaken   int64   `json:"time_taken"`
	AvgSpeed    float64 `json:"avg_speed"`
}

// CancelResult contains metadata about a cancelled download so callers
// can handle event emission and cleanup without the pool needing to import
// the events package (which would create an import cycle).
type CancelResult struct {
	Filename  string
	DestPath  string
	Found     bool
	Completed bool
	WasQueued bool
}
