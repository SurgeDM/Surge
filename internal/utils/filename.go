package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"
	"github.com/kennygrant/sanitize"
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

	// Changing flow to determine candidate filename first

	var candidate string

	// 1. Content-Disposition
	if _, name, err := httpheader.ContentDisposition(resp.Header); err == nil && name != "" {
		candidate = name
		if verbose {
			fmt.Fprintf(os.Stderr, "Filename from Content-Disposition: %s\n", candidate)
		}
	}

	// 2. Query Parameters (if no Content-Disposition)
	if candidate == "" {
		q := parsed.Query()
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

	if sanitizedBecameExtensionOnly(candidate, filename) {
		filename = ""
	}

	if filename == "" || filename == "." || filename == "/" || filename == "_" {
		filename = "download.bin"
		if verbose {
			fmt.Fprintln(os.Stderr, "Falling back to default filename: download.bin")
		}
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
	// The kennygrant/sanitize package replaces invalid characters,
	// handles control characters, and performs general filename safety.
	// We retain some basic fallback logic for absolute basics.

	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)

	if name == "." || name == "/" || name == "\\" {
		return "_"
	}

	return sanitize.Name(name)
}
