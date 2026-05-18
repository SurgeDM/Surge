package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	debugFile *os.File
	debugOnce sync.Once
	logsDir   atomic.Value // string
)

// ConfigureDebug sets the directory for debug logs
func ConfigureDebug(dir string) {
	logsDir.Store(dir)
}

// IsLoggingEnabled returns true if debug logging is configured
// This allows callers to skip expensive argument evaluation
func IsLoggingEnabled() bool {
	val := logsDir.Load()
	if val == nil {
		return false
	}
	dir, ok := val.(string)
	return ok && dir != ""
}

// Debug writes a message to debug.log file in the configured directory
func Debug(format string, args ...any) {
	// Internal fast path check without lock
	val := logsDir.Load()
	if val == nil {
		return
	}
	dir := val.(string)
	if dir == "" {
		return
	}

	// Calculate timestamp only if we are actually logging
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Ensure file is open (still needs once, but fast after first time)
	debugOnce.Do(func() {
		_ = os.MkdirAll(dir, 0o755)
		debugFile, _ = os.Create(filepath.Join(dir, fmt.Sprintf("debug-%s.log", time.Now().Format("20060102-150405"))))
	})

	if debugFile != nil {
		_, _ = fmt.Fprintf(debugFile, "[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	}
}

// CleanupLogs removes old log files, keeping only the most recent retentionCount files
func CleanupLogs(retentionCount int) {
	if retentionCount < 0 {
		return // Keep all logs
	}

	val := logsDir.Load()
	if val == nil {
		return
	}
	dir := val.(string)

	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// If directory doesn't exist, nothing to clean
		return
	}

	var logs []fs.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "debug-") && strings.HasSuffix(entry.Name(), ".log") {
			logs = append(logs, entry)
		}
	}

	// Sort by modification time (newest first)
	// Filenames have timestamp: debug-YYYYMMDD-HHMMSS.log, so alphabetical sort is also chronological
	// But let's rely on ModTime to be safe if possible, or just name since it is consistent
	sort.Slice(logs, func(i, j int) bool {
		// Newest first
		// Since names are YYYYMMDD-HHMMSS, reverse alphabetical works
		return logs[i].Name() > logs[j].Name()
	})

	if len(logs) <= retentionCount {
		return
	}

	// Remove older logs
	for _, log := range logs[retentionCount:] {
		path := filepath.Join(dir, log.Name())
		_ = os.Remove(path)
	}
}
