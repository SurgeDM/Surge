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

// InferFilenameFromURL guesses the filename from a URL string, checking
// query parameters first, then the URL path.
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
		// Reject obvious traversal or multi-component paths
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

// GetUniqueFilename creates a unique filename by appending (1), (2), etc.
// It checks both the actual filesystem and an optional active downloads checker.
func GetUniqueFilename(dir, filename string, isNameActive func(string, string) bool) string {
	if filename == "" {
		return filename
	}

	// Ensure filename is a single path component to avoid directory traversal.
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return filename
	}
	// Normalize to base name and verify that no directory components were present.
	base := filepath.Base(filename)
	if base != filename {
		filename = base
	}
	// Reject any remaining obvious traversal or separator usage.
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || filename == "." || filename == ".." {
		// Unsafe filename; refuse to guess and let caller handle empty result.
		return ""
	}

	existsOnDisk := func(name string) bool {
		targetPath := filepath.Join(dir, name)
		if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
			return true
		}
		// Also check for incomplete download file (.surge extension)
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

	// File exists, generate unique name
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	// Check if name already has a counter like "file(1)"
	base := name
	counter := 1

	// Clean name to ensure parsing works even with trailing spaces
	cleanName := strings.TrimSpace(name)
	if len(cleanName) > 3 && cleanName[len(cleanName)-1] == ')' {
		if openParen := strings.LastIndexByte(cleanName, '('); openParen != -1 {
			// Try to parse number between parens
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

	return fmt.Sprintf("%s(%d)%s", base, counter+100, ext)
}

// GetCategoryPath resolves the destination path based on the filename and configured categories.
// If category routing is disabled or no category matches, it returns the provided default dir.
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
			path := utils.EnsureAbsPath(catPath)
			if err := os.MkdirAll(path, 0o755); err != nil {
				return defaultDir, fmt.Errorf("failed to create category path %s: %w", path, err)
			}
			return path, nil
		}
	}

	return defaultDir, nil
}

// getBaseFilename returns the filename according to the strict priority:
// 1. User defined filename (candidateFilename)
// 2. Probe result (handles Content-Disposition, Query Parameters, ZIP Headers, etc.)
// 3. Inference from URL
func getBaseFilename(url, candidate string, probe *ProbeResult) string {
	if candidate != "" {
		return candidate
	}
	if probe != nil && probe.Filename != "" {
		return probe.Filename
	}
	return InferFilenameFromURL(url)
}

// ResolveDestination determines the final, unique destination path and filename for a download.
// It combines URL inference, category routing, and unique filename generation.
// It returns (final_destination_path, final_filename, error)
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

	return destPath, finalFilename, nil
}

// RemoveIncompleteFile removes the partial .surge file for a given destination path.
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
