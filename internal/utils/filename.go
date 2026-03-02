package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/h2non/filetype"
	"github.com/vfaronov/httpheader"
)

// DetermineFilename extracts the filename from a URL and HTTP response,
// applying various heuristics. It returns the determined filename,
// a new io.Reader that includes any sniffed header bytes, and an error.
func DetermineFilename(rawurl string, resp *http.Response, verbose bool) (string, io.Reader, error) {
	parsed, err := url.Parse(rawurl)
	if err != nil {
		return "", nil, err
	}
	sourceURL := parsed
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		sourceURL = resp.Request.URL
	}

	// Changing flow to determine candidate filename first

	var candidate string

	// 1. Content-Disposition
	if name := contentDispositionFilename(resp.Header); name != "" {
		candidate = name
		if verbose {
			fmt.Fprintf(os.Stderr, "Filename from Content-Disposition: %s\n", candidate)
		}
	}

	// 2. Query Parameters (if no Content-Disposition)
	if candidate == "" {
		q := sourceURL.Query()
		if name := q.Get("filename"); name != "" {
			candidate = name
			if verbose {
				fmt.Fprintf(os.Stderr, "Filename from query param 'filename': %s\n", candidate)
			}
		} else if name := q.Get("file"); name != "" {
			candidate = name
			if verbose {
				fmt.Fprintf(os.Stderr, "Filename from query param 'file': %s\n", candidate)
			}
		}
	}

	// 3. URL Path
	if candidate == "" {
		candidate = filepath.Base(sourceURL.Path)
	}

	filename := sanitizeFilename(candidate)

	header := make([]byte, 512)
	n, rerr := io.ReadFull(resp.Body, header)
	if rerr != nil {
		if rerr == io.ErrUnexpectedEOF || rerr == io.EOF {
			header = header[:n]
		} else {
			return "", nil, fmt.Errorf("reading header: %w", rerr)
		}
	} else {
		header = header[:n]
	}

	body := io.MultiReader(bytes.NewReader(header), resp.Body)

	if verbose {
		mimeType := http.DetectContentType(header)
		fmt.Fprintln(os.Stderr, "Detected MIME:", mimeType)

		if kind, _ := filetype.Match(header); kind != filetype.Unknown {
			fmt.Fprintln(os.Stderr, "Magic Type:", kind.Extension, kind.MIME)
		}
	}

	if candidate == "." && len(header) >= 4 && bytes.HasPrefix(header, []byte{0x50, 0x4B, 0x03, 0x04}) && len(header) >= 30 {
		nameLen := int(binary.LittleEndian.Uint16(header[26:28]))
		start := 30
		end := start + nameLen
		if end <= len(header) {
			zipName := string(header[start:end])
			if zipName != "" {
				filename = filepath.Base(zipName)
				if verbose {
					fmt.Fprintln(os.Stderr, "ZIP internal filename:", zipName)
				}
			}
		}
	}

	if filepath.Ext(filename) == "" {
		if kind, _ := filetype.Match(header); kind != filetype.Unknown {
			if kind.Extension != "" {
				filename = filename + "." + kind.Extension
				if verbose {
					fmt.Fprintf(os.Stderr, "Added extension from magic type: %s\n", kind.Extension)
				}
			}
		}
	}

	if filename == "" || filename == "." || filename == "/" || filename == "\\" || filename == "_" {
		filename = "download.bin"
		if verbose {
			fmt.Fprintln(os.Stderr, "Falling back to default filename: download.bin")
		}
	}

	return filename, body, nil
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func contentDispositionFilename(headers http.Header) string {
	if _, name, err := httpheader.ContentDisposition(headers); err == nil && name != "" {
		return name
	}

	raw := headers.Get("Content-Disposition")
	if raw == "" {
		return ""
	}

	if _, params, err := mime.ParseMediaType(raw); err == nil {
		if extValue := strings.TrimSpace(params["filename*"]); extValue != "" {
			if decoded, ok := decodeRFC5987(extValue); ok && decoded != "" {
				return decoded
			}
			return trimQuoted(extValue)
		}
		if value := strings.TrimSpace(params["filename"]); value != "" {
			return trimQuoted(value)
		}
	}

	if extValue := dispositionParam(raw, "filename*"); extValue != "" {
		if decoded, ok := decodeRFC5987(extValue); ok && decoded != "" {
			return decoded
		}
		return trimQuoted(extValue)
	}
	return trimQuoted(dispositionParam(raw, "filename"))
}

func dispositionParam(raw, key string) string {
	parts := splitDispositionParams(raw)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq == -1 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		if !strings.EqualFold(name, key) {
			continue
		}
		return strings.TrimSpace(part[eq+1:])
	}
	return ""
}

func splitDispositionParams(raw string) []string {
	var parts []string
	var b strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range raw {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			b.WriteRune(r)
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
			b.WriteRune(r)
		case r == ';' && !inQuotes:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	parts = append(parts, b.String())
	return parts
}

func decodeRFC5987(value string) (string, bool) {
	value = trimQuoted(value)
	if value == "" {
		return "", false
	}

	parts := strings.SplitN(value, "'", 3)
	encodedValue := value
	if len(parts) == 3 {
		encodedValue = parts[2]
	}

	decoded, err := url.PathUnescape(encodedValue)
	if err != nil {
		return "", false
	}
	return decoded, true
}

func trimQuoted(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.ReplaceAll(value, `\"`, `"`)
}

func sanitizeFilename(name string) string {
	// Replace backslashes with forward slashes first so filepath.Base treats them as separators
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." {
		return name
	}
	if name == "/" || name == "\\" {
		return "_"
	}
	name = strings.TrimSpace(name)

	// Remove ANSI escape codes
	name = ansiRegex.ReplaceAllString(name, "")

	// Remove control characters
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	name = strings.ReplaceAll(name, "/", "_")
	// Additional standard replacements for windows/linux safety
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	return name
}
