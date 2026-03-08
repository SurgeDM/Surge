package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadURLsFromFile reads URLs from a file.
// Accepts one URL per line or whitespace-separated URLs, and ignores comments.
func ReadURLsFromFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var urls []string
	scanner := bufio.NewScanner(file)
	// 64KB initial, 1MB max
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		urls = append(urls, strings.Fields(line)...)
	}
	return urls, scanner.Err()
}
