package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
	"github.com/vfaronov/httpheader"
)

// DetermineFilename extracts the filename from a URL and HTTP response,
// applying various heuristics. It returns the determined filename,
// a new io.Reader that includes any sniffed header bytes, and an error.
func DetermineFilename(rawurl string, resp *http.Response) (string, io.Reader, error) {
	parsed, err := url.Parse(rawurl)
	if err != nil {
		return "", nil, err
	}

	// Changing flow to determine candidate filename first

	var candidate string

	// 1. Content-Disposition
	if _, name, err := httpheader.ContentDisposition(resp.Header); err == nil && name != "" {
		candidate = name
		Debug("Filename from Content-Disposition: %s", candidate)
	}

	// 2. Query Parameters (if no Content-Disposition)
	if candidate == "" {
		q := parsed.Query()
		if name := q.Get("filename"); name != "" {
			candidate = name
			Debug("Filename from query param 'filename': %s", candidate)
		} else if name := q.Get("file"); name != "" {
			candidate = name
			Debug("Filename from query param 'file': %s", candidate)
		}
	}

	// 3. URL Path
	if candidate == "" {
		candidate = filepath.Base(parsed.Path)
	}

	filename := sanitizeFilename(candidate)
	if sanitizedBecameExtensionOnly(candidate, filename) {
		filename = ""
	}

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

	kind, _ := filetype.Match(header)

	if IsLoggingEnabled() {
		mimeType := http.DetectContentType(header)
		Debug("Detected MIME: %s", mimeType)

		if kind != filetype.Unknown {
			Debug("Magic Type: %s %s", kind.Extension, kind.MIME)
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
				Debug("ZIP internal filename: %s", zipName)
			}
		}
	}

	if filepath.Ext(filename) == "" {
		if kind != filetype.Unknown && kind.Extension != "" {
			filename = filename + "." + kind.Extension
			Debug("Added extension from magic type: %s", kind.Extension)
		}
	}

	if sanitizedBecameExtensionOnly(candidate, filename) {
		filename = ""
	}

	if filename == "" || filename == "." || filename == "/" || filename == "_" {
		filename = "download.bin"
		Debug("Falling back to default filename: download.bin")
	}

	return filename, body, nil
}

func sanitizedBecameExtensionOnly(original, sanitized string) bool {
	sanitizedBase := filepath.Base(strings.TrimSpace(sanitized))
	if sanitizedBase == "" || !strings.HasPrefix(sanitizedBase, ".") || filepath.Ext(sanitizedBase) != sanitizedBase {
		return false
	}

	originalBase := filepath.Base(strings.TrimSpace(original))
	if originalBase == "" || originalBase == "." || originalBase == "/" {
		return true
	}
	return !strings.HasPrefix(originalBase, ".")
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)

	if name == "." || name == "/" || name == "\\" || name == "" {
		return "_"
	}

	// Remove invalid characters for Windows/Linux/macOS
	for _, ch := range []string{"<", ">", ":", "\"", "|", "?", "*"} {
		name = strings.ReplaceAll(name, ch, "_")
	}

	// Trim leading/trailing spaces and trailing periods (problematic on Windows)
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")

	// Remove unprintable control characters
	var b strings.Builder
	for _, c := range name {
		if (c >= 32 && c < 127) || c > 159 { // Keep printable ASCII and non-control Unicode (skip C1 control chars U+0080–U+009F)
			b.WriteRune(c)
		}
	}
	name = b.String()
	name = strings.TrimSpace(name)

	if name == "" {
		return "_"
	}

	return name
}
