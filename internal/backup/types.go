package backup

import (
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

const (
	SchemaVersion   = 1
	BundleExtension = ".surge-export"
)

// Controller captures the lifecycle operations backup needs to produce a stable snapshot.
type Controller interface {
	List() ([]types.DownloadStatus, error)
	Pause(id string) error
	Resume(id string) error
}

type ExportOptions struct {
	IncludeLogs     bool   `json:"include_logs"`
	IncludePartials bool   `json:"include_partials"`
	LeavePaused     bool   `json:"leave_paused"`
	AppVersion      string `json:"app_version,omitempty"`
}

type ImportOptions struct {
	RootDir   string `json:"root_dir,omitempty"`
	Replace   bool   `json:"replace"`
	SessionID string `json:"session_id,omitempty"`
}

type Manifest struct {
	SchemaVersion             int               `json:"schema_version"`
	CreatedAt                 time.Time         `json:"created_at"`
	SurgeVersion              string            `json:"surge_version"`
	OriginalDefaultDownloadDir string           `json:"original_default_download_dir"`
	IncludeLogs               bool              `json:"include_logs"`
	IncludePartials           bool              `json:"include_partials"`
	Counts                    map[string]int    `json:"counts"`
	Files                     []ManifestFile    `json:"files,omitempty"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type PortableDownload struct {
	ID           string               `json:"id"`
	URL          string               `json:"url"`
	DestPath     string               `json:"dest_path"`
	Filename     string               `json:"filename"`
	Status       string               `json:"status"`
	OriginalStatus string             `json:"original_status,omitempty"`
	TotalSize    int64                `json:"total_size"`
	Downloaded   int64                `json:"downloaded"`
	CompletedAt  int64                `json:"completed_at,omitempty"`
	TimeTaken    int64                `json:"time_taken,omitempty"`
	AvgSpeed     float64              `json:"avg_speed,omitempty"`
	Mirrors      []string             `json:"mirrors,omitempty"`
	Resumable    *PortableResumeState `json:"resumable,omitempty"`
}

type PortableResumeState struct {
	URLHash         string       `json:"url_hash"`
	CreatedAt       int64        `json:"created_at"`
	PausedAt        int64        `json:"paused_at"`
	Elapsed         int64        `json:"elapsed"`
	Tasks           []types.Task `json:"tasks,omitempty"`
	ChunkBitmap     []byte       `json:"chunk_bitmap,omitempty"`
	ActualChunkSize int64        `json:"actual_chunk_size,omitempty"`
	FileHash        string       `json:"file_hash,omitempty"`
	PartialFile     string       `json:"partial_file,omitempty"`
}

type bundlePayload struct {
	Settings  *config.Settings   `json:"settings"`
	Downloads []PortableDownload `json:"downloads"`
}

type ImportPreview struct {
	SessionID                       string         `json:"session_id,omitempty"`
	Manifest                        *Manifest      `json:"manifest"`
	RootDir                         string         `json:"root_dir"`
	ImportsByStatus                 map[string]int `json:"imports_by_status"`
	DuplicatesSkipped               int            `json:"duplicates_skipped"`
	RenamedItems                    int            `json:"renamed_items"`
	ResumableJobsDowngradedToQueue  int            `json:"resumable_jobs_downgraded_to_queue"`
	RebasedPaths                    int            `json:"rebased_paths"`
	ExternalizedPaths               int            `json:"externalized_paths"`
	Conflicts                       []ImportConflict `json:"conflicts,omitempty"`
}

type ImportConflict struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	OldPath string `json:"old_path,omitempty"`
	NewPath string `json:"new_path,omitempty"`
	Message string `json:"message,omitempty"`
}

type ImportResult struct {
	Preview      *ImportPreview `json:"preview"`
	Imported     int            `json:"imported"`
	SettingsSaved bool          `json:"settings_saved"`
}

