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

func pathWithinRoot(root, candidate string) bool {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	cleanCandidate, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleanCandidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func sanitizeRelativeExternalPath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	parts := strings.FieldsFunc(cleaned, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	safeParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		safeParts = append(safeParts, part)
	}
	if len(safeParts) == 0 {
		return "imported"
	}
	return filepath.Join(safeParts...)
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
		joined := filepath.Join(targetRoot, cleanOriginal)
		if pathWithinRoot(targetRoot, joined) {
			return pathRebaseResult{
				Path:    joined,
				Rebased: true,
			}
		}
		return pathRebaseResult{
			Path:         filepath.Join(targetRoot, "external", "relative", sanitizeRelativeExternalPath(cleanOriginal)),
			Externalized: true,
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
