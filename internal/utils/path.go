package utils

import (
	"path/filepath"
	"strings"
)

// EnsureAbsPath takes a clean path and forces it to be absolute.
// If it fails to get absolute path (rare), it checks if it's already absolute,
// otherwise relies on the input.
func EnsureAbsPath(path string) string {
	if path == "" {
		path = "."
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// IsWindowsAbsPath reports whether p looks like a Windows absolute path even
// when running on a non-Windows host (for example inside Docker on Linux).
func IsWindowsAbsPath(p string) bool {
	p = strings.TrimSpace(p)
	if len(p) >= 3 &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) &&
		p[1] == ':' &&
		(p[2] == '/' || p[2] == '\\') {
		return true
	}

	// UNC paths (Windows-only; // is POSIX-legal and must not be excluded).
	if len(p) >= 2 && p[0] == '\\' && p[1] == '\\' {
		return true
	}

	return false
}

// MapWindowsPathToDefaultDir projects a Windows absolute client path onto a
// server-side default download directory by preserving only the suffix beneath
// the matching root folder name. Example:
//
//	C:/Users/me/Downloads/subdir -> /downloads/subdir
func MapWindowsPathToDefaultDir(requestPath, defaultDir string) (string, bool) {
	requestPath = strings.TrimSpace(requestPath)
	defaultDir = strings.TrimSpace(defaultDir)
	if requestPath == "" || defaultDir == "" || !IsWindowsAbsPath(requestPath) {
		return "", false
	}

	defaultBase := strings.TrimSpace(filepath.Base(filepath.Clean(defaultDir)))
	if defaultBase == "" || defaultBase == "." || defaultBase == string(filepath.Separator) {
		return "", false
	}

	normalized := strings.ReplaceAll(requestPath, "\\", "/")
	parts := strings.Split(normalized, "/")

	match := -1
	for i, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), defaultBase) {
			match = i
			break
		}
	}
	if match == -1 {
		return "", false
	}

	relParts := make([]string, 0, len(parts)-match-1)
	for _, part := range parts[match+1:] {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", false
		}
		relParts = append(relParts, part)
	}

	if len(relParts) == 0 {
		return filepath.Clean(defaultDir), true
	}

	joined := append([]string{filepath.Clean(defaultDir)}, relParts...)
	return filepath.Join(joined...), true
}
