package backup

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SurgeDM/Surge/internal/utils"
)

type pathRebaseResult struct {
	Path         string
	Rebased      bool
	Externalized bool
}

func sanitizeExternalRoot(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	volume := filepath.VolumeName(cleaned)
	if volume != "" {
		trimmed := strings.TrimSuffix(volume, ":")
		if trimmed != "" {
			return trimmed
		}
	}
	if filepath.IsAbs(cleaned) {
		return "root"
	}
	return "relative"
}

func trimVolumePrefix(path string) string {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return path
	}
	trimmed := strings.TrimPrefix(path, volume)
	trimmed = strings.TrimLeft(trimmed, string(filepath.Separator))
	return trimmed
}

func rebaseImportedPath(originalPath, exportedRoot, targetRoot string) pathRebaseResult {
	targetRoot = utils.EnsureAbsPath(strings.TrimSpace(targetRoot))
	if targetRoot == "" {
		targetRoot = "."
	}

	cleanOriginal := filepath.Clean(strings.TrimSpace(originalPath))
	cleanExportedRoot := filepath.Clean(strings.TrimSpace(exportedRoot))
	if cleanOriginal == "." || cleanOriginal == "" {
		return pathRebaseResult{Path: targetRoot}
	}

	if !filepath.IsAbs(cleanOriginal) {
		return pathRebaseResult{
			Path:    filepath.Join(targetRoot, cleanOriginal),
			Rebased: true,
		}
	}

	if cleanExportedRoot != "." && cleanExportedRoot != "" {
		if rel, err := filepath.Rel(cleanExportedRoot, cleanOriginal); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return pathRebaseResult{
				Path:    filepath.Join(targetRoot, rel),
				Rebased: true,
			}
		}
	}

	externalRoot := sanitizeExternalRoot(cleanOriginal)
	rest := trimVolumePrefix(cleanOriginal)
	rest = strings.TrimLeft(rest, string(filepath.Separator))
	if runtime.GOOS != "windows" && strings.HasPrefix(rest, "/") {
		rest = strings.TrimLeft(rest, "/")
	}

	return pathRebaseResult{
		Path:         filepath.Join(targetRoot, "external", externalRoot, rest),
		Externalized: true,
	}
}

