package utils

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	jsonURLFieldNames = []string{"url", "link", "uri", "download_url"}
	jsonURLListNames  = []string{"urls", "links", "downloads"}
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

	input := bufio.NewReader(file)
	if bom, err := input.Peek(3); err == nil && bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = input.Discard(3)
	}

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
		err = readCSVURLs(input, addURL)
	case ".json":
		err = readJSONURLs(input, addURL)
	default:
		err = readTextURLs(input, addURL)
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
	row := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to parse CSV: %w", err)
		}
		row++

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
			if urlColumn >= len(record) {
				return fmt.Errorf("CSV row %d is missing URL column %d", row, urlColumn+1)
			}
			addURL(record[urlColumn])
			continue
		}
		if len(record) == 1 {
			addURL(record[0])
			continue
		}
		for _, field := range record {
			field = strings.TrimSpace(field)
			lowerField := strings.ToLower(field)
			if strings.HasPrefix(lowerField, "http://") || strings.HasPrefix(lowerField, "https://") {
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
		for _, listName := range jsonURLListNames {
			for _, key := range matchingJSONKeys(value, listName) {
				if collectJSONURLs(value[key], addURL) {
					found = true
				}
			}
		}
	}
	return found
}

func jsonURLField(object map[string]any) string {
	for _, fieldName := range jsonURLFieldNames {
		for _, key := range matchingJSONKeys(object, fieldName) {
			if url, ok := object[key].(string); ok && strings.TrimSpace(url) != "" {
				return url
			}
		}
	}
	return ""
}

func matchingJSONKeys(object map[string]any, normalizedName string) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		if normalizeJSONFieldName(key) == normalizedName {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func isURLFieldName(name string) bool {
	normalized := normalizeJSONFieldName(name)
	for _, fieldName := range jsonURLFieldNames {
		if normalized == fieldName {
			return true
		}
	}
	return false
}

func normalizeJSONFieldName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return strings.ReplaceAll(normalized, " ", "_")
}
