package processing

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// ChecksumResult holds the outcome of a checksum verification.
type ChecksumResult struct {
	Algorithm string
	Expected  string
	Actual    string
	Match     bool
}

// VerifyChecksum computes the hash of a file and compares it to the expected value.
// algorithm should be one of: md5, sha1, sha256.
// expected should be a hex-encoded hash string.
func VerifyChecksum(filePath string, algorithm string, expected string) (*ChecksumResult, error) {
	if filePath == "" || algorithm == "" || expected == "" {
		return nil, fmt.Errorf("filepath, algorithm, and expected hash are all required")
	}

	algorithm = strings.ToLower(algorithm)
	expected = strings.ToLower(strings.TrimSpace(expected))

	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha1", "sha-1":
		algorithm = "sha1"
		h = sha1.New()
	case "sha256", "sha-256":
		algorithm = "sha256"
		h = sha256.New()
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm: %s", algorithm)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("failed to read file for checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	return &ChecksumResult{
		Algorithm: algorithm,
		Expected:  expected,
		Actual:    actual,
		Match:     actual == expected,
	}, nil
}

// ParseDigestHeader parses an HTTP Digest header (RFC 3230) and returns
// the algorithm and hex-encoded hash. Returns empty strings if not parseable.
// Example header: "sha-256=base64hash" or "SHA-256=base64hash"
func ParseDigestHeader(header string) (algorithm string, hexHash string) {
	parts := strings.SplitN(header, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}

	algo := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])

	switch algo {
	case "sha-256":
		algo = "sha256"
	case "sha-1":
		algo = "sha1"
	case "md5":
		// no normalization needed
	default:
		return "", ""
	}

	// Some servers provide hex directly in Digest; prefer that when it matches
	// the expected hash length for the selected algorithm.
	expectedHexLen := 0
	switch algo {
	case "md5":
		expectedHexLen = 32
	case "sha1":
		expectedHexLen = 40
	case "sha256":
		expectedHexLen = 64
	}
	if len(value) == expectedHexLen {
		if _, err := hex.DecodeString(value); err == nil {
			return algo, strings.ToLower(value)
		}
	}

	// RFC 3230 uses base64 (padded or unpadded, standard or URL-safe)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(value); err == nil {
			h := hex.EncodeToString(decoded)
			if len(h) == expectedHexLen {
				return algo, h
			}
		}
	}

	// Hex fallback - only accept if length matches the expected hash size
	if _, err := hex.DecodeString(value); err == nil && len(value) == expectedHexLen {
		return algo, strings.ToLower(value)
	}
	return "", ""
}
