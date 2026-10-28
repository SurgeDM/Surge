package store

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/google/uuid"
)

const (
	DefaultInlineHashTimeout = 10 * time.Second
	hashPrefixMD5            = "md5:"
	hashPrefixSHA256         = "sha256:"
)

type SaveStateOptions struct {
	// SkipFileHash disables file_hash computation entirely for this save.
	SkipFileHash bool
	// InlineHashTimeout limits synchronous hashing time. If zero or negative, DefaultInlineHashTimeout is used.
	InlineHashTimeout time.Duration
}

type MasterState struct {
	Version   int
	Downloads []types.DownloadRecord
}

// persistedDetailRecord is the explicit gob wire format for per-download
// resume state. It retains only fields needed to reconstruct a paused download,
// including request headers, while excluding runtime handles and output paths.
type persistedDetailRecord struct {
	ID              string
	URLHash         string
	URL             string
	Filename        string
	DestPath        string
	TotalSize       int64
	Downloaded      int64
	Status          string
	Error           string
	CreatedAt       int64
	PausedAt        int64
	CompletedAt     int64
	TimeTaken       int64
	Elapsed         int64
	AvgSpeed        float64
	Tasks           []types.Task
	ChunkBitmap     []byte
	ActualChunkSize int64
	FileHash        string
	Mirrors         []string
	RateLimit       int64
	RateLimitSet    bool
	Workers         int
	MinChunkSize    int64
	Headers         map[string]string
}

func toPersistedDetail(state *types.DownloadRecord) *persistedDetailRecord {
	if state == nil {
		return nil
	}

	return &persistedDetailRecord{
		ID:              state.ID,
		URLHash:         state.URLHash,
		URL:             state.URL,
		Filename:        state.Filename,
		DestPath:        state.DestPath,
		TotalSize:       state.TotalSize,
		Downloaded:      state.Downloaded,
		Status:          state.Status,
		Error:           state.Error,
		CreatedAt:       state.CreatedAt,
		PausedAt:        state.PausedAt,
		CompletedAt:     state.CompletedAt,
		TimeTaken:       state.TimeTaken,
		Elapsed:         state.Elapsed,
		AvgSpeed:        state.AvgSpeed,
		Tasks:           append([]types.Task(nil), state.Tasks...),
		ChunkBitmap:     append([]byte(nil), state.ChunkBitmap...),
		ActualChunkSize: state.ActualChunkSize,
		FileHash:        state.FileHash,
		Mirrors:         append([]string(nil), state.Mirrors...),
		RateLimit:       state.RateLimit,
		RateLimitSet:    state.RateLimitSet,
		Workers:         state.Workers,
		MinChunkSize:    state.MinChunkSize,
		Headers:         maps.Clone(state.Headers),
	}
}

func fromPersistedDetail(state *persistedDetailRecord) *types.DownloadRecord {
	if state == nil {
		return nil
	}

	return &types.DownloadRecord{
		ID:              state.ID,
		URLHash:         state.URLHash,
		URL:             state.URL,
		Filename:        state.Filename,
		DestPath:        state.DestPath,
		TotalSize:       state.TotalSize,
		Downloaded:      state.Downloaded,
		Status:          state.Status,
		Error:           state.Error,
		CreatedAt:       state.CreatedAt,
		PausedAt:        state.PausedAt,
		CompletedAt:     state.CompletedAt,
		TimeTaken:       state.TimeTaken,
		Elapsed:         state.Elapsed,
		AvgSpeed:        state.AvgSpeed,
		Tasks:           append([]types.Task(nil), state.Tasks...),
		ChunkBitmap:     append([]byte(nil), state.ChunkBitmap...),
		ActualChunkSize: state.ActualChunkSize,
		FileHash:        state.FileHash,
		Mirrors:         append([]string(nil), state.Mirrors...),
		RateLimit:       state.RateLimit,
		RateLimitSet:    state.RateLimitSet,
		Workers:         state.Workers,
		MinChunkSize:    state.MinChunkSize,
		Headers:         maps.Clone(state.Headers),
	}
}

type DetailState struct {
	Version int
	State   *persistedDetailRecord
}

func URLHash(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:8])
}

func getMasterPath() string {
	return filepath.Join(baseDir, "master.gob")
}

func getDetailPath(dir, id string) (string, error) {
	if !filepath.IsLocal(id) || strings.ContainsAny(id, "/\\:\x00") || id == "." {
		return "", fmt.Errorf("invalid download ID %q", id)
	}
	return filepath.Join(dir, "details", id+".gob"), nil
}

func SaveState(url string, destPath string, state *types.DownloadRecord) error {
	return SaveStateWithOptions(url, destPath, state, SaveStateOptions{})
}

func SaveStateWithOptions(url string, destPath string, state *types.DownloadRecord, opts SaveStateOptions) error {
	if state.ID == "" {
		state.ID = uuid.New().String()
	}

	state.URLHash = URLHash(url)
	state.PausedAt = time.Now().Unix()
	if state.CreatedAt == 0 {
		state.CreatedAt = time.Now().Unix()
	}

	hashTimeout := opts.InlineHashTimeout
	if hashTimeout <= 0 {
		hashTimeout = DefaultInlineHashTimeout
	}
	if opts.SkipFileHash {
		state.FileHash = ""
	} else {
		fileHash, timedOut, err := computeFileHashMD5WithTimeout(state.DestPath+types.IncompleteSuffix, hashTimeout)
		if err != nil {
			utils.Debug("SaveState: skipping file hash for %s due to error: %v", state.DestPath, err)
		} else if timedOut {
			utils.Debug("SaveState: skipping file hash for %s after timeout %v", state.DestPath, hashTimeout)
		}
		state.FileHash = fileHash
	}

	if state.ChunkBitmap == nil {
		state.ChunkBitmap = []byte{}
	}
	if state.Mirrors == nil {
		state.Mirrors = []string{}
	}
	if state.Tasks == nil {
		state.Tasks = []types.Task{}
	}

	ds := DetailState{
		Version: 2,
		State:   toPersistedDetail(state),
	}

	if err := ensureDirs(); err != nil {
		return err
	}

	// Acquire the write lock for both the detail write and the master list update.
	// Snapshot baseDir here so both operations use the same directory — eliminating
	// the race window that existed when baseDir was re-read under a separate RLock
	// after ensureDirs() had already released its own RLock.
	masterMu.Lock()
	defer masterMu.Unlock()

	dir := baseDir
	if dir == "" {
		return fmt.Errorf("state backend not configured")
	}

	detailPath, err := getDetailPath(dir, state.ID)
	if err != nil {
		return err
	}
	if err := atomicWrite(detailPath, ds); err != nil {
		return fmt.Errorf("failed to write detail state: %w", err)
	}

	// Update the lightweight index (MasterList)
	list, err := loadMasterListUnlocked()
	if err != nil {
		return fmt.Errorf("failed to load master list: %w", err)
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == state.ID {
			list.Downloads[i].URL = state.URL
			list.Downloads[i].DestPath = state.DestPath
			list.Downloads[i].Filename = state.Filename
			list.Downloads[i].TotalSize = state.TotalSize
			list.Downloads[i].Downloaded = state.Downloaded
			list.Downloads[i].TimeTaken = state.Elapsed / int64(time.Millisecond)
			list.Downloads[i].Workers = state.Workers
			list.Downloads[i].MinChunkSize = state.MinChunkSize
			if err := saveMasterListLocked(list); err != nil {
				return fmt.Errorf("failed to update master list: %w", err)
			}
			found = true
			break
		}
	}
	if !found {
		utils.Debug("SaveState: ID %s not found in master list; call AddToMasterList first", state.ID)
	}

	return nil
}

func computeFileHashMD5WithTimeout(path string, timeout time.Duration) (string, bool, error) {
	if timeout <= 0 {
		timeout = DefaultInlineHashTimeout
	}
	if timeout < time.Microsecond {
		return "", true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()

	h := md5.New()
	buf := make([]byte, utils.MiB)
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return "", true, nil
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if _, err := h.Write(buf[:n]); err != nil {
				return "", false, fmt.Errorf("failed to update md5 hash: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", false, fmt.Errorf("failed to read file for md5 hash: %w", readErr)
		}
	}

	return hashPrefixMD5 + hex.EncodeToString(h.Sum(nil)), false, nil
}

func compareAgainstStoredFileHash(path string, storedHash string) (bool, error) {
	algo, expected := parseStoredHash(storedHash)
	switch algo {
	case "md5":
		current, timedOut, err := computeFileHashMD5WithTimeout(path, DefaultInlineHashTimeout)
		if err != nil {
			return false, err
		}
		if timedOut {
			utils.Debug("Integrity: hash timed out for %s, skipping verification", path)
			return true, nil
		}
		return strings.EqualFold(strings.TrimPrefix(current, hashPrefixMD5), expected), nil
	case "sha256":
		current, err := computeFileHash(path)
		if err != nil {
			return false, err
		}
		return strings.EqualFold(current, expected), nil
	default:
		return false, fmt.Errorf("unsupported hash format: %q", storedHash)
	}
}

func parseStoredHash(storedHash string) (algo, value string) {
	trimmed := strings.TrimSpace(storedHash)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, hashPrefixMD5):
		return "md5", trimmed[len(hashPrefixMD5):]
	case strings.HasPrefix(lower, hashPrefixSHA256):
		return "sha256", trimmed[len(hashPrefixSHA256):]
	case len(trimmed) == 32:
		return "md5", trimmed
	case len(trimmed) == 64:
		return "sha256", trimmed
	default:
		return "", trimmed
	}
}

func LoadState(url string, destPath string) (*types.DownloadRecord, error) {
	masterList, err := LoadMasterList()
	if err != nil {
		return nil, err
	}

	var foundID string

	for _, e := range masterList.Downloads {
		if e.URL == url && e.DestPath == destPath && e.Status != "completed" {
			foundID = e.ID
			break
		}
	}

	if foundID == "" {
		return nil, fmt.Errorf("state not found: %w", os.ErrNotExist)
	}

	masterMu.RLock()
	dir := baseDir
	masterMu.RUnlock()

	var ds DetailState
	detailPath, err := getDetailPath(dir, foundID)
	if err != nil {
		return nil, err
	}
	if err := loadGob(detailPath, &ds); err != nil {
		return nil, fmt.Errorf("failed to load detail state: %w", err)
	}

	state := fromPersistedDetail(ds.State)
	if state != nil && state.ChunkBitmap == nil {
		state.ChunkBitmap = []byte{}
	}

	return state, nil
}

func LoadStates(ids []string) (map[string]*types.DownloadRecord, error) {
	states := make(map[string]*types.DownloadRecord)
	var errs []error

	masterMu.RLock()
	dir := baseDir
	masterMu.RUnlock()

	for _, id := range ids {
		detailPath, err := getDetailPath(dir, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var ds DetailState
		if err := loadGob(detailPath, &ds); err != nil {
			if !os.IsNotExist(err) {
				utils.Debug("LoadStates: failed to load state for %s: %v", id, err)
				errs = append(errs, fmt.Errorf("id %s: %w", id, err))
			}
			continue
		}
		if state := fromPersistedDetail(ds.State); state != nil {
			if state.ChunkBitmap == nil {
				state.ChunkBitmap = []byte{}
			}
			states[id] = state
		}
	}
	return states, errors.Join(errs...)
}

func DeleteState(id string) error {
	masterMu.Lock()
	defer masterMu.Unlock()

	detailPath, err := getDetailPath(baseDir, id)
	if err != nil {
		return err
	}
	// Remove detail file, propagating real errors (but not "file not found").
	if err := utils.RemoveFile(detailPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove detail state: %w", err)
	}

	// Also remove from the master list so LoadState won't find an orphaned index entry.
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	out := make([]types.DownloadRecord, 0, len(list.Downloads))
	for _, e := range list.Downloads {
		if e.ID != id {
			out = append(out, e)
		}
	}
	list.Downloads = out
	return saveMasterListLocked(list)
}

func DeleteTasks(id string) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	var ds DetailState
	detailPath, err := getDetailPath(baseDir, id)
	if err != nil {
		return err
	}
	if err := loadGob(detailPath, &ds); err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to clear
		}
		return fmt.Errorf("failed to load detail state: %w", err)
	}
	if ds.State == nil {
		return nil
	}
	ds.State.Tasks = []types.Task{}
	if err := atomicWrite(detailPath, ds); err != nil {
		return fmt.Errorf("failed to clear tasks: %w", err)
	}
	return nil
}

func LoadMasterList() (*types.MasterList, error) {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return nil, err
	}
	if baseDir == "" {
		return list, nil
	}
	if _, err := os.Stat(getMasterPath()); err == nil {
		if err := saveMasterListLocked(list); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

func sanitizeMasterDownload(download *types.DownloadRecord) {
	download.Headers = nil
	download.ProgressCh = nil
	download.ProgressState = nil
	download.Runtime = nil
	download.Limiter = nil
	download.IsResume = false
	download.IsExplicitCategory = false
	download.SupportsRange = false
}

func saveMasterListLocked(list *types.MasterList) error {
	if baseDir == "" {
		return fmt.Errorf("state backend not configured")
	}
	if err := ensureDirsLocked(); err != nil {
		return err
	}
	downloads := make([]types.DownloadRecord, len(list.Downloads))
	for index, download := range list.Downloads {
		sanitizeMasterDownload(&download)
		downloads[index] = download
	}
	ms := MasterState{
		Version:   2,
		Downloads: downloads,
	}
	return atomicWrite(getMasterPath(), ms)
}

func AddToMasterList(entry types.DownloadRecord) error {
	masterMu.Lock()
	defer masterMu.Unlock()

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Mirrors == nil {
		entry.Mirrors = []string{}
	}

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == entry.ID {
			entry.CategoryOverride = e.CategoryOverride
			entry.CategoryAssigned = e.CategoryAssigned
			entry.PendingCategoryMove = e.PendingCategoryMove
			if e.Status == "completed" {
				entry.DestPath = e.DestPath
			}
			if entry.Status == "completed" && entry.PendingCategoryMove != "" {
				target := filepath.Join(entry.PendingCategoryMove, filepath.Base(entry.DestPath))
				if target == entry.DestPath {
					entry.PendingCategoryMove = ""
				} else {
					moveErr := os.MkdirAll(entry.PendingCategoryMove, 0755)
					if moveErr == nil {
						moveErr = copyCategoryFile(entry.DestPath, target)
					}
					if moveErr != nil {
						entry.CategoryMoveError = moveErr.Error()
					} else {
						oldPath := entry.DestPath
						entry.DestPath = target
						entry.PendingCategoryMove = ""
						entry.CategoryMoveError = ""
						list.Downloads[i] = entry
						if err := saveMasterListLocked(list); err != nil {
							_ = os.Remove(target)
							return err
						}
						if err := os.Remove(oldPath); err != nil {
							return fmt.Errorf("moved file, but old copy remains: %w", err)
						}
						return nil
					}
				}
			}
			list.Downloads[i] = entry
			found = true
			break
		}
	}
	if !found {
		list.Downloads = append(list.Downloads, entry)
	}
	return saveMasterListLocked(list)
}

func loadMasterListUnlocked() (*types.MasterList, error) {
	return loadMasterListWithUnsupportedVersionCleanupUnlocked(true)
}

func loadMasterListReadOnlyUnlocked() (*types.MasterList, error) {
	return loadMasterListWithUnsupportedVersionCleanupUnlocked(false)
}

func loadMasterListWithUnsupportedVersionCleanupUnlocked(cleanupUnsupported bool) (*types.MasterList, error) {
	if baseDir == "" {
		return &types.MasterList{Downloads: []types.DownloadRecord{}}, nil
	}
	var ms MasterState
	if err := loadGob(getMasterPath(), &ms); err != nil {
		if os.IsNotExist(err) {
			return &types.MasterList{Downloads: []types.DownloadRecord{}}, nil
		}
		return nil, err
	}
	if ms.Version != 2 {
		if cleanupUnsupported {
			utils.Debug("Master list has unsupported version %d (expected 2), deleting to start fresh", ms.Version)
			_ = os.Remove(getMasterPath())
		} else {
			utils.Debug("Master list has unsupported version %d (expected 2), leaving it untouched during read-only access", ms.Version)
		}
		return &types.MasterList{Downloads: []types.DownloadRecord{}}, nil
	}
	for index := range ms.Downloads {
		sanitizeMasterDownload(&ms.Downloads[index])
		if ms.Downloads[index].CategoryAssigned && ms.Downloads[index].CategoryOverride == nil {
			empty := ""
			ms.Downloads[index].CategoryOverride = &empty
		}
	}
	return &types.MasterList{Downloads: ms.Downloads}, nil
}

func RemoveFromMasterList(id string) error {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	out := []types.DownloadRecord{}
	for _, e := range list.Downloads {
		if e.ID != id {
			out = append(out, e)
		}
	}
	list.Downloads = out
	return saveMasterListLocked(list)
}

func GetDownload(id string) (*types.DownloadRecord, error) {
	masterMu.RLock()
	defer masterMu.RUnlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return nil, err
	}
	for _, e := range list.Downloads {
		if e.ID == id {
			return &e, nil
		}
	}
	return nil, nil
}

func LoadPausedDownloads() ([]types.DownloadRecord, error) {
	list, err := LoadMasterList()
	if err != nil {
		return nil, err
	}
	var paused []types.DownloadRecord
	for _, e := range list.Downloads {
		if e.Status == "paused" || e.Status == "queued" {
			paused = append(paused, e)
		}
	}
	return paused, nil
}

func LoadCompletedDownloads() ([]types.DownloadRecord, error) {
	list, err := LoadMasterList()
	if err != nil {
		return nil, err
	}
	var completed []types.DownloadRecord
	for _, e := range list.Downloads {
		if e.Status == "completed" {
			completed = append(completed, e)
		}
	}
	return completed, nil
}

func CheckDownloadExists(url string) (bool, error) {
	list, err := LoadMasterList()
	if err != nil {
		return false, err
	}
	for _, e := range list.Downloads {
		if e.URL == url {
			return true, nil
		}
	}
	return false, nil
}

func UpdateStatus(id string, status string) error {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == id {
			list.Downloads[i].Status = status
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return saveMasterListLocked(list)
}

func UpdateURL(id string, newURL string) error {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == id {
			list.Downloads[i].URL = newURL
			list.Downloads[i].URLHash = URLHash(newURL)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return saveMasterListLocked(list)
}

func PauseAllDownloads() error {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i, e := range list.Downloads {
		if e.Status != "completed" {
			list.Downloads[i].Status = "paused"
		}
	}
	return saveMasterListLocked(list)
}

func ResumeAllDownloads() error {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i, e := range list.Downloads {
		if e.Status == "paused" {
			list.Downloads[i].Status = "queued"
		}
	}
	return saveMasterListLocked(list)
}

func ListAllDownloads() ([]types.DownloadRecord, error) {
	masterMu.RLock()
	defer masterMu.RUnlock()

	list, err := loadMasterListReadOnlyUnlocked()
	if err != nil {
		return nil, err
	}
	return list.Downloads, nil
}

func RemoveCompletedDownloads() (int64, error) {
	return removeDownloadsByStatus("completed")
}

func RemoveFailedDownloads() (int64, error) {
	return removeDownloadsByStatus("failed")
}

func removeDownloadsByStatus(status string) (int64, error) {
	masterMu.Lock()
	defer masterMu.Unlock()

	list, err := loadMasterListUnlocked()
	if err != nil {
		return 0, err
	}
	detailPaths := make([]string, len(list.Downloads))
	for i, entry := range list.Downloads {
		if entry.Status != status {
			continue
		}
		detailPaths[i], err = getDetailPath(baseDir, entry.ID)
		if err != nil {
			return 0, err
		}
	}
	var count int64
	out := []types.DownloadRecord{}
	for i, e := range list.Downloads {
		if e.Status == status {
			count++
			_ = utils.RemoveFile(detailPaths[i])
		} else {
			out = append(out, e)
		}
	}
	list.Downloads = out
	err = saveMasterListLocked(list)
	return count, err
}
func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// UpdateRateLimit updates the rate limit of a download in the database
func UpdateRateLimit(id string, rate int64) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == id {
			list.Downloads[i].RateLimit = rate
			list.Downloads[i].RateLimitSet = true
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return saveMasterListLocked(list)
}

// UpdateDefaultRateLimit updates the inherited rate limit of a download
// only if the download does not have a user-set override.
func UpdateDefaultRateLimit(id string, rate int64) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i, e := range list.Downloads {
		if e.ID == id {
			if e.RateLimitSet {
				return nil // user override takes precedence, no write needed
			}
			list.Downloads[i].RateLimit = rate
			return saveMasterListLocked(list)
		}
	}
	return fmt.Errorf("%w: %s", types.ErrNotFound, id)
}

// ClearRateLimit removes a download-specific rate limit override.
func ClearRateLimit(id string) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i, e := range list.Downloads {
		if e.ID == id {
			list.Downloads[i].RateLimit = 0
			list.Downloads[i].RateLimitSet = false
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", types.ErrNotFound, id)
	}
	return saveMasterListLocked(list)
}

// NormalizeStaleDownloads resets downloads that were interrupted (e.g., app crash)
// and appear as dead/frozen items in the TUI.
func NormalizeStaleDownloads() (int, error) {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return 0, err
	}
	count := 0
	for i, e := range list.Downloads {
		if e.Status == "downloading" {
			list.Downloads[i].Status = "paused"
			count++
		}
	}
	if count > 0 {
		err = saveMasterListLocked(list)
	}
	return count, err
}

// ValidateIntegrity checks that paused .surge files still exist and haven't been tampered with.
// Removes orphaned or corrupted entries from the master list.
// Returns the number of entries removed.
func ValidateIntegrity() (int, error) {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return 0, err
	}
	detailPaths := make([]string, len(list.Downloads))
	for i, entry := range list.Downloads {
		detailPaths[i], err = getDetailPath(baseDir, entry.ID)
		if err != nil {
			return 0, err
		}
	}

	removed := 0
	expectedSurgePaths := make(map[string]struct{})
	candidateDirs := make(map[string]struct{})

	var out []types.DownloadRecord

	for i, e := range list.Downloads {
		detailPath := detailPaths[i]
		if e.DestPath == "" {
			out = append(out, e)
			continue
		}

		candidateDirs[filepath.Dir(e.DestPath)] = struct{}{}
		if e.Status != "completed" {
			expectedSurgePaths[e.DestPath+types.IncompleteSuffix] = struct{}{}
		}

		if e.Status == "paused" || e.Status == "queued" {
			surgePath := e.DestPath + types.IncompleteSuffix

			// Check if .surge file exists
			_, statErr := os.Stat(surgePath)
			if os.IsNotExist(statErr) {
				// File missing - remove orphaned DB entry
				utils.Debug("Integrity: .surge file missing for %s, removing entry %s", e.DestPath, e.ID)
				_ = utils.RemoveFile(detailPath)
				removed++
				continue
			}
			if statErr != nil {
				return removed, fmt.Errorf("failed to stat %s: %w", surgePath, statErr)
			}

			// If we have a stored hash, verify it
			var ds DetailState
			if err := loadGob(detailPath, &ds); err == nil && ds.State != nil && ds.State.FileHash != "" {
				matches, err := compareAgainstStoredFileHash(surgePath, ds.State.FileHash)
				if err != nil {
					return removed, fmt.Errorf("failed to verify hash for %s: %w", surgePath, err)
				}
				if !matches {
					// File has been tampered with - remove entry and corrupted file
					utils.Debug("Integrity: hash mismatch for %s (expected %s), removing", surgePath, ds.State.FileHash)
					_ = utils.RemoveFile(surgePath)
					_ = utils.RemoveFile(detailPath)
					removed++
					continue
				}
			}
		}

		out = append(out, e)
	}

	list.Downloads = out
	if removed > 0 {
		if err := saveMasterListLocked(list); err != nil {
			return removed, err
		}
	}

	// Remove orphan .surge files that no longer have matching entries.
	for dir := range candidateDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("failed to read directory %s: %w", dir, err)
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasSuffix(name, types.IncompleteSuffix) {
				continue
			}
			surgePath := filepath.Join(dir, name)
			if _, ok := expectedSurgePaths[surgePath]; ok {
				continue
			}
			_ = utils.RemoveFile(surgePath)
			utils.Debug("Integrity: removed orphan .surge file %s", surgePath)
		}
	}

	return removed, nil
}

// SetCategory persists an explicit assignment independently of engine snapshots.
func SetCategory(id, category string) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i := range list.Downloads {
		if list.Downloads[i].ID == id {
			list.Downloads[i].CategoryMoveError = ""
			list.Downloads[i].PendingCategoryMove = ""
			list.Downloads[i].CategoryAssigned = true
			list.Downloads[i].CategoryOverride = &category
			return saveMasterListLocked(list)
		}
	}
	return fmt.Errorf("%w: %s", types.ErrNotFound, id)
}

// MoveCompletedCategory saves the new destination before removing the source.
// Destination creation is exclusive, so existing files are never replaced.
func MoveCompletedCategory(id, category, directory string) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i := range list.Downloads {
		entry := &list.Downloads[i]
		if entry.ID != id {
			continue
		}
		if entry.Status != "completed" {
			return fmt.Errorf("download must complete before moving")
		}
		source := entry.DestPath
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("move failed: file missing: %w", err)
			}
			return fmt.Errorf("move failed: cannot access source: %w", err)
		}
		target := filepath.Join(directory, filepath.Base(source))
		if target == source {
			entry.PendingCategoryMove = ""
			entry.CategoryMoveError = ""
			entry.CategoryAssigned = true
			entry.CategoryOverride = &category
			return saveMasterListLocked(list)
		}
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		if err := copyCategoryFile(source, target); err != nil {
			return fmt.Errorf("move failed: %w", err)
		}
		entry.DestPath = target
		entry.CategoryMoveError = ""
		entry.PendingCategoryMove = ""
		entry.CategoryAssigned = true
		entry.CategoryOverride = &category
		if err := saveMasterListLocked(list); err != nil {
			_ = os.Remove(target)
			return err
		}
		if err := os.Remove(source); err != nil {
			return fmt.Errorf("file saved at %s, but old copy could not be removed: %w", target, err)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", types.ErrNotFound, id)
}

// ScheduleCategoryMove persists a move request without changing an open file.
func ScheduleCategoryMove(id, category, directory string) error {
	masterMu.Lock()
	defer masterMu.Unlock()
	list, err := loadMasterListUnlocked()
	if err != nil {
		return err
	}
	for i := range list.Downloads {
		if list.Downloads[i].ID == id {
			if list.Downloads[i].Status == "completed" {
				return fmt.Errorf("download completed; retry category move")
			}
			list.Downloads[i].CategoryAssigned = true
			list.Downloads[i].CategoryOverride = &category
			list.Downloads[i].CategoryMoveError = ""
			list.Downloads[i].PendingCategoryMove = directory
			return saveMasterListLocked(list)
		}
	}
	return types.ErrNotFound
}

// copyCategoryFile creates a durable destination without overwriting anything.
func copyCategoryFile(source, target string) (err error) {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if err := os.Link(source, target); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
		if err != nil {
			_ = os.Remove(target)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
