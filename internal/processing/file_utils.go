package processing

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

// InferFilenameFromURL is the final naming fallback when neither the user nor
// the probe produced a trustworthy filename.
func InferFilenameFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}

	isSafeComponent := func(name string) bool {
		name = strings.TrimSpace(name)
		if name == "" {
			return false
		}
		// Keep URL-derived names inside the chosen destination directory.
		if strings.Contains(name, "/") || strings.Contains(name, "\\") {
			return false
		}
		if name == "." || name == ".." || name == "/" {
			return false
		}
		// Reject simple Windows absolute paths like C:foo or C:\foo
		if len(name) >= 2 && (name[1] == ':' && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z'))) {
			return false
		}
		return true
	}

	query := parsed.Query()
	if name := strings.TrimSpace(query.Get("filename")); name != "" {
		if base := strings.TrimSpace(path.Base(name)); isSafeComponent(base) {
			return base
		}
	}
	if name := strings.TrimSpace(query.Get("file")); name != "" {
		if base := strings.TrimSpace(path.Base(name)); isSafeComponent(base) {
			return base
		}
	}

	base := strings.TrimSpace(path.Base(parsed.Path))
	if !isSafeComponent(base) {
		return ""
	}
	return base
}

// GetUniqueFilename keeps final files and .surge working files in the same
// collision namespace so concurrent or resumed downloads do not share a path.
func GetUniqueFilename(dir, filename string, isNameActive func(string, string) bool) string {
	if filename == "" {
		return filename
	}

	// Treat every candidate as a single file name so routing cannot escape dir.
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return filename
	}
	baseName := filepath.Base(filename)
	if baseName != filename {
		filename = baseName
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || filename == "." || filename == ".." {
		return ""
	}

	existsOnDisk := func(name string) bool {
		targetPath := filepath.Join(dir, name)
		if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
			return true
		}
		// A .surge sibling means another active or recoverable download already
		// claimed this filename, so we must not hand it out again.
		if _, err := os.Stat(targetPath + types.IncompleteSuffix); !os.IsNotExist(err) {
			return true
		}
		return false
	}

	existsAnywhere := func(name string) bool {
		if isNameActive != nil && isNameActive(dir, name) {
			return true
		}
		return existsOnDisk(name)
	}

	if !existsAnywhere(filename) {
		return filename
	}

	// Keep suffixing from the same base so repeated retries stay deterministic.
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	base := name
	counter := 1

	cleanName := strings.TrimSpace(name)
	if len(cleanName) > 3 && cleanName[len(cleanName)-1] == ')' {
		if openParen := strings.LastIndexByte(cleanName, '('); openParen != -1 {
			numStr := cleanName[openParen+1 : len(cleanName)-1]
			if num, err := strconv.Atoi(numStr); err == nil && num > 0 {
				base = cleanName[:openParen]
				counter = num + 1
			}
		}
	}

	for i := range 100 { // Try next 100 numbers
		candidate := fmt.Sprintf("%s(%d)%s", base, counter+i, ext)
		if !existsAnywhere(candidate) {
			return candidate
		}
	}

	return ""
}

// GetCategoryPath applies category routing only while the caller is still using
// the default destination, so explicit user paths are left untouched.
func GetCategoryPath(filename, defaultDir string, settings *config.Settings) (string, error) {
	if settings == nil || !settings.General.CategoryEnabled || filename == "" {
		return defaultDir, nil
	}

	cat, err := config.GetCategoryForFile(filename, settings.General.Categories)
	if err != nil {
		return defaultDir, fmt.Errorf("category match error for %s: %w", filename, err)
	}

	if cat != nil {
		if catPath := config.ResolveCategoryPath(cat, defaultDir); catPath != "" {
			return utils.EnsureAbsPath(catPath), nil
		}
	}

	return defaultDir, nil
}

// getBaseFilename keeps naming deterministic across retries by preferring the
// most authoritative source available before uniqueness is applied.
func getBaseFilename(url, candidate string, probe *ProbeResult) string {
	if candidate != "" {
		return candidate
	}
	if probe != nil && probe.Filename != "" {
		return probe.Filename
	}
	return InferFilenameFromURL(url)
}

// ResolveDestination centralizes routing and naming so CLI, TUI, and API
// requests all land on the same final path before the engine starts downloading.
func ResolveDestination(url, candidateFilename, defaultDir string, routeToCategory bool, settings *config.Settings, probe *ProbeResult, isNameActive func(string, string) bool) (string, string, error) {
	filename := getBaseFilename(url, candidateFilename, probe)

	destPath := defaultDir
	if routeToCategory && settings != nil && settings.General.CategoryEnabled && filename != "" {
		var err error
		destPath, err = GetCategoryPath(filename, defaultDir, settings)
		if err != nil {
			return "", "", err
		}
	}

	finalFilename := GetUniqueFilename(destPath, filename, isNameActive)
	if finalFilename == "" {
		return "", "", fmt.Errorf("could not determine a unique filename for %s", url)
	}

	return destPath, finalFilename, nil
}

// RemoveIncompleteFile drops only the reserved working file, leaving any
// promoted final file untouched.
func RemoveIncompleteFile(destPath string) error {
	if destPath == "" {
		return nil
	}
	surgePath := destPath + types.IncompleteSuffix
	if err := os.Remove(surgePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
