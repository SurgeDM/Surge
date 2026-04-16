package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/google/uuid"
)

type plannedImport struct {
	Record      PortableDownload
	FinalID     string
	FinalPath   string
	FinalStatus string
	Skip        bool
	UsePartial  bool
}

func Export(ctx context.Context, w io.Writer, opts ExportOptions, controller Controller) (*Manifest, error) {
	if w == nil {
		return nil, fmt.Errorf("export writer is required")
	}

	resumeIDs, err := quiesceActiveDownloads(ctx, controller)
	if err != nil {
		return nil, err
	}
	if len(resumeIDs) > 0 && !opts.LeavePaused && controller != nil {
		defer resumeDownloads(controller, resumeIDs)
	}

	settings, err := config.LoadSettings()
	if err != nil || settings == nil {
		settings = config.DefaultSettings()
	}

	records, counts, err := exportDownloads(opts.IncludePartials)
	if err != nil {
		return nil, err
	}

	settingsBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	downloadsBytes, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, err
	}

	type fileSource struct {
		name string
		size int64
		sum  string
		data []byte
		path string
	}

	var files []fileSource
	addBytesSource := func(name string, data []byte) {
		sum := sha256.Sum256(data)
		files = append(files, fileSource{
			name: name,
			size: int64(len(data)),
			sum:  hex.EncodeToString(sum[:]),
			data: data,
		})
	}

	addFileSource := func(name, path string) error {
		sum, size, err := fileSHA256(path)
		if err != nil {
			return err
		}
		files = append(files, fileSource{
			name: name,
			size: size,
			sum:  sum,
			path: path,
		})
		return nil
	}

	addBytesSource("settings/settings.json", settingsBytes)
	addBytesSource("state/downloads.json", downloadsBytes)

	if opts.IncludePartials {
		for _, rec := range records {
			if rec.Resumable == nil || rec.Resumable.PartialFile == "" {
				continue
			}
			sourcePath := rec.DestPath + types.IncompleteSuffix
			if err := addFileSource(rec.Resumable.PartialFile, sourcePath); err != nil {
				return nil, err
			}
		}
	}

	if opts.IncludeLogs {
		logFiles, err := collectLogFiles()
		if err != nil {
			return nil, err
		}
		for _, path := range logFiles {
			relName := filepath.ToSlash(filepath.Join("logs", filepath.Base(path)))
			if err := addFileSource(relName, path); err != nil {
				return nil, err
			}
		}
	}

	manifest := &Manifest{
		SchemaVersion:              SchemaVersion,
		CreatedAt:                  time.Now().UTC(),
		SurgeVersion:               strings.TrimSpace(opts.AppVersion),
		OriginalDefaultDownloadDir: settings.General.DefaultDownloadDir,
		IncludeLogs:                opts.IncludeLogs,
		IncludePartials:            opts.IncludePartials,
		Counts:                     counts,
	}
	for _, file := range files {
		manifest.Files = append(manifest.Files, ManifestFile{
			Path:   filepath.ToSlash(file.name),
			SHA256: file.sum,
			Size:   file.size,
		})
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}

	zw := zip.NewWriter(w)
	if err := writeZipBytes(zw, "manifest.json", manifestBytes); err != nil {
		_ = zw.Close()
		return nil, err
	}
	for _, file := range files {
		if len(file.data) > 0 {
			if err := writeZipBytes(zw, file.name, file.data); err != nil {
				_ = zw.Close()
				return nil, err
			}
			continue
		}
		if err := writeZipFile(zw, file.name, file.path); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return manifest, nil
}

func PreviewImport(ctx context.Context, r io.Reader, opts ImportOptions) (*ImportPreview, error) {
	_ = ctx
	opened, err := openBundle(r)
	if err != nil {
		return nil, err
	}
	defer opened.Close()

	manifest, payload, err := loadBundle(opened.Reader)
	if err != nil {
		return nil, err
	}

	return buildImportPreview(manifest, payload, opts), nil
}

func ApplyImport(ctx context.Context, r io.Reader, opts ImportOptions, controller Controller) (*ImportResult, error) {
	opened, err := openBundle(r)
	if err != nil {
		return nil, err
	}
	defer opened.Close()

	manifest, payload, err := loadBundle(opened.Reader)
	if err != nil {
		return nil, err
	}

	resumeIDs, err := quiesceActiveDownloads(ctx, controller)
	if err != nil {
		return nil, err
	}
	if len(resumeIDs) > 0 && controller != nil {
		defer resumeDownloads(controller, resumeIDs)
	}

	preview := buildImportPreview(manifest, payload, opts)
	rootDir := preview.RootDir
	plan, err := planImport(payload.Downloads, manifest, opts, rootDir)
	if err != nil {
		return nil, err
	}

	if opts.Replace {
		if err := clearExistingState(); err != nil {
			return nil, err
		}
	}

	settingsSaved := false
	if payload.Settings != nil {
		if payload.Settings.General.DefaultDownloadDir != "" && rootDir != "" {
			payload.Settings.General.DefaultDownloadDir = utils.EnsureAbsPath(rootDir)
		}
		if err := config.SaveSettings(payload.Settings); err != nil {
			return nil, err
		}
		settingsSaved = true
	}

	logsRestored, err := restoreLogFiles(opened.Reader, manifest, opts.Replace)
	if err != nil {
		return nil, err
	}

	imported := 0
	for _, item := range plan {
		if item.Skip {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		record := item.Record
		entry := types.DownloadEntry{
			ID:          item.FinalID,
			URL:         record.URL,
			URLHash:     state.URLHash(record.URL),
			DestPath:    item.FinalPath,
			Filename:    filepath.Base(item.FinalPath),
			Status:      item.FinalStatus,
			TotalSize:   record.TotalSize,
			CompletedAt: record.CompletedAt,
			TimeTaken:   record.TimeTaken,
			AvgSpeed:    record.AvgSpeed,
			Mirrors:     append([]string(nil), record.Mirrors...),
		}

		switch item.FinalStatus {
		case "completed":
			entry.Downloaded = record.TotalSize
		case "error":
			entry.Downloaded = record.Downloaded
		default:
			if item.UsePartial {
				entry.Downloaded = record.Downloaded
			} else {
				entry.Downloaded = 0
			}
		}

		if item.UsePartial && record.Resumable != nil {
			if err := restorePartialFile(opened.Reader, record.Resumable.PartialFile, item.FinalPath+types.IncompleteSuffix, rootDir); err != nil {
				return nil, err
			}

			saved := &types.DownloadState{
				ID:              item.FinalID,
				URLHash:         record.Resumable.URLHash,
				URL:             record.URL,
				DestPath:        item.FinalPath,
				TotalSize:       record.TotalSize,
				Downloaded:      record.Downloaded,
				Tasks:           append([]types.Task(nil), record.Resumable.Tasks...),
				Filename:        filepath.Base(item.FinalPath),
				CreatedAt:       record.Resumable.CreatedAt,
				PausedAt:        record.Resumable.PausedAt,
				Elapsed:         record.Resumable.Elapsed,
				Mirrors:         append([]string(nil), record.Mirrors...),
				ChunkBitmap:     append([]byte(nil), record.Resumable.ChunkBitmap...),
				ActualChunkSize: record.Resumable.ActualChunkSize,
				FileHash:        record.Resumable.FileHash,
			}
			if err := state.SaveStateWithOptions(record.URL, item.FinalPath, saved, state.SaveStateOptions{SkipFileHash: true}); err != nil {
				return nil, err
			}
			if item.FinalStatus != "paused" {
				if err := state.UpdateStatus(item.FinalID, item.FinalStatus); err != nil {
					return nil, err
				}
			}
		} else {
			if err := state.AddToMasterList(entry); err != nil {
				return nil, err
			}
		}

		imported++
	}

	return &ImportResult{
		Preview:       preview,
		Imported:      imported,
		SettingsSaved: settingsSaved,
		LogsRestored:  logsRestored,
	}, nil
}

func exportDownloads(includePartials bool) ([]PortableDownload, map[string]int, error) {
	entries, err := state.ListAllDownloads()
	if err != nil {
		return nil, nil, err
	}

	counts := make(map[string]int)
	out := make([]PortableDownload, 0, len(entries))
	for _, entry := range entries {
		rec := PortableDownload{
			ID:            entry.ID,
			URL:           entry.URL,
			DestPath:      entry.DestPath,
			Filename:      entry.Filename,
			Status:        entry.Status,
			OriginalStatus: entry.Status,
			TotalSize:     entry.TotalSize,
			Downloaded:    entry.Downloaded,
			CompletedAt:   entry.CompletedAt,
			TimeTaken:     entry.TimeTaken,
			AvgSpeed:      entry.AvgSpeed,
			Mirrors:       append([]string(nil), entry.Mirrors...),
		}

		switch entry.Status {
		case "completed", "error":
		default:
			saved, _ := state.LoadState(entry.URL, entry.DestPath)
			if includePartials && saved != nil {
				partialPath := entry.DestPath + types.IncompleteSuffix
				if _, err := os.Stat(partialPath); err == nil {
					rec.Resumable = &PortableResumeState{
						URLHash:         saved.URLHash,
						CreatedAt:       saved.CreatedAt,
						PausedAt:        saved.PausedAt,
						Elapsed:         saved.Elapsed,
						Tasks:           append([]types.Task(nil), saved.Tasks...),
						ChunkBitmap:     append([]byte(nil), saved.ChunkBitmap...),
						ActualChunkSize: saved.ActualChunkSize,
						FileHash:        saved.FileHash,
						PartialFile:     filepath.ToSlash(filepath.Join("partials", entry.ID+types.IncompleteSuffix)),
					}
					switch entry.Status {
					case "downloading", "pausing":
						rec.Status = "paused"
					}
					break
				}
			}

			if entry.Status == "paused" || entry.Status == "downloading" || entry.Status == "pausing" {
				rec.Status = "queued"
				rec.Downloaded = 0
			}
		}

		counts[rec.Status]++
		out = append(out, rec)
	}

	slices.SortFunc(out, func(a, b PortableDownload) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, counts, nil
}

func buildImportPreview(manifest *Manifest, payload *bundlePayload, opts ImportOptions) *ImportPreview {
	rootDir := utils.EnsureAbsPath(strings.TrimSpace(opts.RootDir))
	if rootDir == "" {
		settings, _ := config.LoadSettings()
		if settings != nil {
			rootDir = utils.EnsureAbsPath(strings.TrimSpace(settings.General.DefaultDownloadDir))
		}
		if rootDir == "" {
			rootDir = "."
		}
	}

	plan, conflicts := planImportPreview(payload.Downloads, manifest, opts, rootDir)

	preview := &ImportPreview{
		Manifest:        manifest,
		RootDir:         rootDir,
		ImportsByStatus: make(map[string]int),
		Conflicts:       conflicts,
	}
	for _, item := range plan {
		if item.Skip {
			preview.DuplicatesSkipped++
			continue
		}
		preview.ImportsByStatus[item.FinalStatus]++
		if item.FinalPath != "" && item.Record.DestPath != "" && filepath.Clean(item.FinalPath) != filepath.Clean(rebaseImportedPath(item.Record.DestPath, manifest.OriginalDefaultDownloadDir, rootDir).Path) {
			preview.RenamedItems++
		}
		if item.Record.OriginalStatus != "" && item.Record.OriginalStatus != item.FinalStatus && item.FinalStatus == "queued" {
			preview.ResumableJobsDowngradedToQueue++
		}
		rebase := rebaseImportedPath(item.Record.DestPath, manifest.OriginalDefaultDownloadDir, rootDir)
		if rebase.Rebased {
			preview.RebasedPaths++
		}
		if rebase.Externalized {
			preview.ExternalizedPaths++
		}
	}

	return preview
}

func planImport(downloads []PortableDownload, manifest *Manifest, opts ImportOptions, rootDir string) ([]plannedImport, error) {
	plan, _ := planImportPreview(downloads, manifest, opts, rootDir)
	return plan, nil
}

func planImportPreview(downloads []PortableDownload, manifest *Manifest, opts ImportOptions, rootDir string) ([]plannedImport, []ImportConflict) {
	existing, _ := state.ListAllDownloads()
	existingByID := make(map[string]types.DownloadEntry, len(existing))
	existingByLogical := make(map[string]types.DownloadEntry, len(existing))
	reservedNames := make(map[string]struct{})
	if !opts.Replace {
		for _, entry := range existing {
			existingByID[entry.ID] = entry
			existingByLogical[logicalKey(entry.URL, entry.DestPath, entry.Filename, entry.Status)] = entry
			if entry.DestPath != "" {
				reservedNames[pathKey(filepath.Dir(entry.DestPath), filepath.Base(entry.DestPath))] = struct{}{}
			}
		}
	}

	var conflicts []ImportConflict
	plan := make([]plannedImport, 0, len(downloads))
	for _, record := range downloads {
		rebase := rebaseImportedPath(record.DestPath, manifest.OriginalDefaultDownloadDir, rootDir)
		finalPath := rebase.Path
		finalID := record.ID
		finalStatus := record.Status
		usePartial := record.Resumable != nil && strings.TrimSpace(record.Resumable.PartialFile) != ""

		if !usePartial && finalStatus == "paused" {
			finalStatus = "queued"
		}

		if existing, ok := existingByID[record.ID]; ok && !sameLogical(existing, record, finalPath) {
			oldID := finalID
			finalID = uuid.New().String()
			conflicts = append(conflicts, ImportConflict{
				Type:    "id_conflict",
				ID:      oldID,
				Message: "assigned a new download ID during import",
			})
		}

		if existing, ok := existingByLogical[logicalKey(record.URL, finalPath, filepath.Base(finalPath), finalStatus)]; ok && existing.ID != record.ID {
			plan = append(plan, plannedImport{
				Record:      record,
				FinalID:     finalID,
				FinalPath:   finalPath,
				FinalStatus: finalStatus,
				Skip:        true,
			})
			conflicts = append(conflicts, ImportConflict{
				Type:    "duplicate",
				ID:      existing.ID,
				Path:    finalPath,
				Message: "skipped exact duplicate import record",
			})
			continue
		}

		dir := filepath.Dir(finalPath)
		name := filepath.Base(finalPath)
		uniqueName := processing.GetUniqueFilename(dir, name, func(checkDir, checkName string) bool {
			_, exists := reservedNames[pathKey(checkDir, checkName)]
			return exists
		})
		if uniqueName != "" && uniqueName != name {
			newPath := filepath.Join(dir, uniqueName)
			conflicts = append(conflicts, ImportConflict{
				Type:    "path_rename",
				ID:      record.ID,
				OldPath: finalPath,
				NewPath: newPath,
				Message: "renamed during import to avoid destination collision",
			})
			finalPath = newPath
		}

		if finalPath != "" {
			reservedNames[pathKey(filepath.Dir(finalPath), filepath.Base(finalPath))] = struct{}{}
		}

		plan = append(plan, plannedImport{
			Record:      record,
			FinalID:     finalID,
			FinalPath:   finalPath,
			FinalStatus: finalStatus,
			UsePartial:  usePartial,
		})
	}

	return plan, conflicts
}

func sameLogical(entry types.DownloadEntry, record PortableDownload, finalPath string) bool {
	return entry.URL == record.URL &&
		filepath.Clean(entry.DestPath) == filepath.Clean(finalPath) &&
		entry.Filename == filepath.Base(finalPath) &&
		entry.Status == record.Status
}

func logicalKey(url, destPath, filename, status string) string {
	return strings.Join([]string{
		strings.TrimSpace(url),
		filepath.Clean(strings.TrimSpace(destPath)),
		strings.TrimSpace(filename),
		strings.TrimSpace(status),
	}, "|")
}

func pathKey(dir, name string) string {
	return filepath.Clean(dir) + "|" + strings.TrimSpace(name)
}

func quiesceActiveDownloads(ctx context.Context, controller Controller) ([]string, error) {
	if controller == nil {
		return nil, nil
	}
	statuses, err := controller.List()
	if err != nil {
		return nil, err
	}

	var activeIDs []string
	for _, status := range statuses {
		if status.Status == "downloading" || status.Status == "pausing" {
			activeIDs = append(activeIDs, status.ID)
		}
	}
	for _, id := range activeIDs {
		if err := controller.Pause(id); err != nil {
			return nil, err
		}
	}
	if len(activeIDs) == 0 {
		return nil, nil
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		statuses, err := controller.List()
		if err != nil {
			return nil, err
		}
		pending := false
		for _, status := range statuses {
			if !slices.Contains(activeIDs, status.ID) {
				continue
			}
			if status.Status == "downloading" || status.Status == "pausing" {
				pending = true
				break
			}
		}
		if !pending {
			return activeIDs, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	return activeIDs, nil
}

func resumeDownloads(controller Controller, ids []string) {
	for _, id := range ids {
		if err := controller.Resume(id); err != nil {
			utils.Debug("backup: failed resuming %s after transfer: %v", id, err)
		}
	}
}

func collectLogFiles() ([]string, error) {
	logDir := config.GetLogsDir()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		out = append(out, filepath.Join(logDir, entry.Name()))
	}
	slices.Sort(out)
	return out, nil
}

type openedBundle struct {
	File   *os.File
	Reader *zip.Reader
}

func (o *openedBundle) Close() error {
	if o == nil || o.File == nil {
		return nil
	}
	name := o.File.Name()
	if err := o.File.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

func openBundle(r io.Reader) (*openedBundle, error) {
	if r == nil {
		return nil, fmt.Errorf("bundle reader is required")
	}
	tmpFile, err := os.CreateTemp("", "surge-import-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tmpFile.Close()
	}()
	if _, err := io.Copy(tmpFile, r); err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmpFile.Name())
		return nil, err
	}
	return &openedBundle{File: file, Reader: reader}, nil
}

func loadBundle(reader *zip.Reader) (*Manifest, *bundlePayload, error) {
	manifestBytes, err := readZipEntry(reader, "manifest.json")
	if err != nil {
		return nil, nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, nil, err
	}
	if manifest.SchemaVersion != SchemaVersion {
		return nil, nil, fmt.Errorf("unsupported bundle schema version %d", manifest.SchemaVersion)
	}
	if err := verifyManifestFiles(reader, manifest.Files); err != nil {
		return nil, nil, err
	}

	settingsBytes, err := readZipEntry(reader, "settings/settings.json")
	if err != nil {
		return nil, nil, err
	}
	var settings config.Settings
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		return nil, nil, err
	}
	downloadsBytes, err := readZipEntry(reader, "state/downloads.json")
	if err != nil {
		return nil, nil, err
	}
	var downloads []PortableDownload
	if err := json.Unmarshal(downloadsBytes, &downloads); err != nil {
		return nil, nil, err
	}
	return &manifest, &bundlePayload{
		Settings:  &settings,
		Downloads: downloads,
	}, nil
}

func readZipEntry(reader *zip.Reader, name string) ([]byte, error) {
	for _, file := range reader.File {
		if filepath.ToSlash(file.Name) != filepath.ToSlash(name) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("bundle entry %s not found", name)
}

func verifyManifestFiles(reader *zip.Reader, files []ManifestFile) error {
	for _, file := range files {
		data, err := readZipEntry(reader, file.Path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != file.SHA256 {
			return fmt.Errorf("checksum mismatch for %s", file.Path)
		}
		if int64(len(data)) != file.Size {
			return fmt.Errorf("size mismatch for %s", file.Path)
		}
	}
	return nil
}

func writeZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeZipFile(zw *zip.Writer, name, path string) error {
	w, err := zw.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(w, file)
	return err
}

func fileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func normalizeAllowedRoot(root string) (string, error) {
	root = utils.EnsureAbsPath(strings.TrimSpace(root))
	if root == "" {
		if wd, err := filepath.Abs("."); err == nil {
			root = wd
		}
	}
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("invalid allowed root")
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	return cleanRoot, nil
}

func resolveRestoreDestination(destPath, allowedRoot string) (string, error) {
	finalDest := filepath.Clean(strings.TrimSpace(destPath))
	if finalDest == "" || finalDest == "." {
		return "", fmt.Errorf("invalid destination path")
	}

	cleanDest, err := filepath.Abs(finalDest)
	if err != nil {
		return "", err
	}
	cleanRoot, err := normalizeAllowedRoot(allowedRoot)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(cleanRoot, cleanDest)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write outside allowed root")
	}

	return cleanDest, nil
}

func restoreBundleFile(reader *zip.Reader, bundlePath, destPath, allowedRoot string) error {
	data, err := readZipEntry(reader, bundlePath)
	if err != nil {
		return err
	}
	finalDest, err := resolveRestoreDestination(destPath, allowedRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(finalDest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(finalDest, data, 0o644)
}

func restorePartialFile(reader *zip.Reader, bundlePath, destPath, allowedRoot string) error {
	return restoreBundleFile(reader, bundlePath, destPath, allowedRoot)
}

func restoreLogFiles(reader *zip.Reader, manifest *Manifest, replace bool) (int, error) {
	logPaths := manifestLogPaths(manifest)
	if len(logPaths) == 0 {
		return 0, nil
	}

	logDir := config.GetLogsDir()
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return 0, err
	}
	if replace {
		if err := clearLogFiles(); err != nil {
			return 0, err
		}
	}

	restored := 0
	for _, bundlePath := range logPaths {
		filename := filepath.Base(filepath.FromSlash(bundlePath))
		if filename == "." || filename == "" {
			continue
		}

		destName := filename
		if !replace {
			destName = processing.GetUniqueFilename(logDir, filename, nil)
			if destName == "" {
				return restored, fmt.Errorf("could not determine a unique log filename for %s", filename)
			}
		}

		if err := restoreBundleFile(reader, bundlePath, filepath.Join(logDir, destName), logDir); err != nil {
			return restored, err
		}
		restored++
	}

	return restored, nil
}

func manifestLogPaths(manifest *Manifest) []string {
	if manifest == nil {
		return nil
	}

	var paths []string
	for _, file := range manifest.Files {
		path := filepath.ToSlash(strings.TrimSpace(file.Path))
		if strings.HasPrefix(path, "logs/") {
			paths = append(paths, path)
		}
	}
	return paths
}

func clearExistingState() error {
	entries, _ := state.ListAllDownloads()
	for _, entry := range entries {
		if entry.Status == "completed" || strings.TrimSpace(entry.DestPath) == "" {
			continue
		}
		_ = os.Remove(entry.DestPath + types.IncompleteSuffix)
	}

	db, err := state.GetDB()
	if err != nil {
		return err
	}
	return withTransaction(db, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM tasks"); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM downloads"); err != nil {
			return err
		}
		return nil
	})
}

func clearLogFiles() error {
	entries, err := os.ReadDir(config.GetLogsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(config.GetLogsDir(), entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func withTransaction(db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
