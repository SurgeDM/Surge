package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadURLsFromFile reads URLs from a file.
// Accepts one URL per line or whitespace-separated URLs, and ignores comments.
// Trailing-slash-only variants are treated as the same URL so batch imports
// behave consistently across CLI and TUI entry points.
func ReadURLsFromFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var urls []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	// 64KB initial, 1MB max
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := -1
		for i := 0; i < len(line); i++ {
			if line[i] == '#' && i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
				idx = i
				break
			}
		}
		if idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		for _, u := range strings.Fields(line) {
			normalized := strings.TrimRight(u, "/")
			if !seen[normalized] {
				seen[normalized] = true
				urls = append(urls, u)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid URLs found in file")
	}
	return urls, nil
}
