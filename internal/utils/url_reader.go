package utils

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadURLsFromFile reads URLs from a text, CSV, or JSON file.
// Text files accept one URL per line or whitespace-separated URLs.
// Lines starting with # are treated as comments, and inline comments
// after whitespace are also supported.
// Empty lines are ignored and duplicate URLs are removed.
// URLs that differ only by a trailing slash are considered duplicates.
func ReadURLsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var urls []string
	seen := make(map[string]bool)
	addURL := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		normalized := strings.TrimRight(url, "/")
		if normalized == "" {
			normalized = url
		}
		if !seen[normalized] {
			seen[normalized] = true
			urls = append(urls, url)
		}
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		err = readCSVURLs(file, addURL)
	case ".json":
		err = readJSONURLs(file, addURL)
	default:
		err = readTextURLs(file, addURL)
	}
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid URLs found in file")
	}
	return urls, nil
}

func readTextURLs(r io.Reader, addURL func(string)) error {
	scanner := bufio.NewScanner(r)
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
		for _, url := range strings.Fields(line) {
			addURL(url)
		}
	}
	return scanner.Err()
}

func readCSVURLs(r io.Reader, addURL func(string)) error {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	urlColumn := -1
	firstRecord := true
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to parse CSV: %w", err)
		}

		if firstRecord {
			firstRecord = false
			for i, field := range record {
				if isURLFieldName(field) {
					urlColumn = i
					break
				}
			}
			if urlColumn >= 0 {
				continue
			}
		}

		if urlColumn >= 0 {
			if urlColumn < len(record) {
				addURL(record[urlColumn])
			}
			continue
		}
		if len(record) == 1 {
			addURL(record[0])
			continue
		}
		for _, field := range record {
			field = strings.TrimSpace(field)
			if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
				addURL(field)
			}
		}
	}
}

func readJSONURLs(r io.Reader, addURL func(string)) error {
	decoder := json.NewDecoder(r)
	var document any
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("failed to parse JSON: multiple JSON values")
		}
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	if !collectJSONURLs(document, addURL) {
		return fmt.Errorf("no URL entries found in JSON file")
	}
	return nil
}

func collectJSONURLs(value any, addURL func(string)) bool {
	found := false
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			switch item := item.(type) {
			case string:
				addURL(item)
				found = true
			case map[string]any:
				if url := jsonURLField(item); url != "" {
					addURL(url)
					found = true
				}
			}
		}
	case map[string]any:
		if url := jsonURLField(value); url != "" {
			addURL(url)
			return true
		}
		for key, nested := range value {
			switch strings.ToLower(key) {
			case "urls", "links", "downloads":
				if collectJSONURLs(nested, addURL) {
					found = true
				}
			}
		}
	}
	return found
}

func jsonURLField(object map[string]any) string {
	for key, value := range object {
		if !isURLFieldName(key) {
			continue
		}
		if url, ok := value.(string); ok {
			return url
		}
	}
	return ""
}

func isURLFieldName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "url", "link", "uri", "download_url":
		return true
	default:
		return false
	}
}
