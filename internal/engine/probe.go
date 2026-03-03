package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
)

var ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) " +
	"Chrome/120.0.0.0 Safari/537.36"

var (
	probeClientOnce sync.Once
	probeClient     *http.Client
)

const metadataProbeTimeout = 10 * time.Second

// ProbeResult contains all metadata from server probe
type ProbeResult struct {
	FileSize      int64
	SupportsRange bool
	Filename      string
	ContentType   string
}

// ProbeServer sends GET with Range: bytes=0-0 to determine server capabilities
// headers is optional - pass nil for non-authenticated probes
func ProbeServer(ctx context.Context, rawurl string, filenameHint string, headers map[string]string) (*ProbeResult, error) {
	utils.Debug("Probing server: %s", rawurl)

	var resp *http.Response
	var err error

	client := getProbeClient()

	// Retry logic for probe request
	for i := 0; i < 3; i++ {
		// Stop if parent context is already done
		if ctx.Err() != nil {
			if err == nil {
				err = fmt.Errorf("probe request aborted: %w", ctx.Err())
			}
			break
		}

		if i > 0 {
			select {
			case <-ctx.Done():
				err = fmt.Errorf("probe request aborted during retry: %w", ctx.Err())
			case <-time.After(1 * time.Second):
			}
			if ctx.Err() != nil {
				break
			}
			utils.Debug("Retrying probe... attempt %d", i+1)
		}

		probeCtx, cancel := context.WithTimeout(ctx, types.ProbeTimeout)

		req, reqErr := newMetadataRequest(probeCtx, http.MethodGet, rawurl, headers)
		if reqErr != nil {
			cancel()
			err = fmt.Errorf("failed to create probe request: %w", reqErr)
			break // Fatal error, don't retry
		}

		req.Header.Set("Range", "bytes=0-0")

		resp, err = client.Do(req)

		// If we get a 403 Forbidden or 405 Method Not Allowed, it might be due to the Range header.
		// Some servers (like NotebookLLM streaming) reject Range requests entirely.
		// We retry once without the Range header.
		if err == nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusMethodNotAllowed) {
			utils.Debug("Probe got %d, retrying without Range header", resp.StatusCode)
			_ = resp.Body.Close() // Close previous response

			reqNoRange, reqNoRangeErr := newMetadataRequest(probeCtx, http.MethodGet, rawurl, headers)
			if reqNoRangeErr != nil {
				cancel()
				err = fmt.Errorf("failed to create probe request without range: %w", reqNoRangeErr)
				break
			}

			resp, err = client.Do(reqNoRange)
		}

		cancel()

		if err == nil {
			break // Success
		}
	}

	if err != nil {
		return nil, fmt.Errorf("probe request failed after retries: %w", err)
	}

	defer func() {
		// Only drain a small amount of data (32KB) to allow connection reuse for small responses (e.g., 206 Partial Content).
		// For large responses (e.g., 200 OK), reading the whole file into discard takes too long.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		_ = resp.Body.Close()
	}()

	utils.Debug("Probe response status: %d", resp.StatusCode)

	result := &ProbeResult{}

	// Determine range support and file size based on status code
	switch resp.StatusCode {
	case http.StatusPartialContent: // 206
		result.SupportsRange = true
		// Parse Content-Range: bytes 0-0/TOTAL
		contentRange := resp.Header.Get("Content-Range")
		utils.Debug("Content-Range header: %s", contentRange)
		if contentRange != "" {
			// Format: "bytes 0-0/12345" or "bytes 0-0/*"
			if idx := strings.LastIndex(contentRange, "/"); idx != -1 {
				sizeStr := contentRange[idx+1:]
				if sizeStr != "*" {
					result.FileSize, _ = strconv.ParseInt(sizeStr, 10, 64)
				}
			}
		}
		utils.Debug("Range supported, file size: %d", result.FileSize)

	case http.StatusOK: // 200 - server ignores Range header
		result.SupportsRange = false
		contentLength := resp.Header.Get("Content-Length")
		if contentLength != "" {
			result.FileSize, _ = strconv.ParseInt(contentLength, 10, 64)
		}
		utils.Debug("Range NOT supported (got 200), file size: %d", result.FileSize)

	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Determine filename using strengthened logic
	name, _, err := utils.DetermineFilename(rawurl, resp, false)
	if err != nil {
		utils.Debug("Error determining filename: %v", err)
		name = "download.bin"
	}
	if filenameHint == "" && isGenericProbeFilename(name) {
		if fallbackName, fallbackErr := probeFilenameWithoutRange(ctx, client, rawurl, headers); fallbackErr != nil {
			utils.Debug("Fallback metadata probe for filename failed: %v", fallbackErr)
		} else if !isGenericProbeFilename(fallbackName) {
			utils.Debug("Fallback metadata probe improved filename: %s -> %s", name, fallbackName)
			name = fallbackName
		}
	}

	if filenameHint != "" {
		result.Filename = filenameHint
	} else {
		result.Filename = name
	}

	result.ContentType = resp.Header.Get("Content-Type")

	utils.Debug("Probe complete - filename: %s, size: %d, range: %v",
		result.Filename, result.FileSize, result.SupportsRange)

	return result, nil
}

func isGenericProbeFilename(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "" ||
		normalized == "." ||
		normalized == "/" ||
		normalized == "\\" ||
		normalized == "_" ||
		normalized == "download" ||
		normalized == "download.bin"
}

func probeFilenameWithoutRange(ctx context.Context, client *http.Client, rawurl string, headers map[string]string) (string, error) {
	metaCtx, cancel := context.WithTimeout(ctx, metadataProbeTimeout)
	defer cancel()

	headReq, err := newMetadataRequest(metaCtx, http.MethodHead, rawurl, headers)
	if err != nil {
		return "", err
	}
	headResp, err := client.Do(headReq)
	if err == nil {
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(headResp.Body, 8*1024))
			_ = headResp.Body.Close()
		}()
		if headResp.StatusCode >= 200 && headResp.StatusCode < 300 {
			name, _, resolveErr := utils.DetermineFilename(rawurl, headResp, false)
			if resolveErr == nil && name != "" {
				return name, nil
			}
		}
	}
	if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(metaCtx.Err(), context.DeadlineExceeded)) {
		return "", fmt.Errorf("metadata HEAD probe timed out: %w", err)
	}
	if metaErr := metaCtx.Err(); metaErr != nil {
		return "", metaErr
	}

	getReq, err := newMetadataRequest(metaCtx, http.MethodGet, rawurl, headers)
	if err != nil {
		return "", err
	}
	getResp, err := client.Do(getReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(metaCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("metadata GET probe timed out: %w", err)
		}
		if metaErr := metaCtx.Err(); metaErr != nil {
			return "", metaErr
		}
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(getResp.Body, 8*1024))
		_ = getResp.Body.Close()
	}()
	if getResp.StatusCode < 200 || getResp.StatusCode >= 300 {
		return "", fmt.Errorf("metadata GET request returned status %d", getResp.StatusCode)
	}

	name, _, err := utils.DetermineFilename(rawurl, getResp, false)
	if err != nil {
		return "", err
	}
	return name, nil
}

func newMetadataRequest(ctx context.Context, method, rawurl string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawurl, nil)
	if err != nil {
		return nil, err
	}
	for key, val := range headers {
		if !strings.EqualFold(key, "Range") {
			req.Header.Set(key, val)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	return req, nil
}

func getProbeClient() *http.Client {
	probeClientOnce.Do(func() {
		// Reuse a single client to share connection pools across probe calls.
		probeClient = &http.Client{
			Timeout: types.ProbeTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				// Preserve headers for authenticated redirect probes.
				if len(via) > 0 {
					for key, vals := range via[0].Header {
						req.Header[key] = vals
					}
				}
				return nil
			},
		}
	})

	return probeClient
}

// ProbeMirrors concurrently checks a list of mirrors and returns valid ones and errors
func ProbeMirrors(ctx context.Context, mirrors []string) (valid []string, errors map[string]error) {
	// Deduplicate
	unique := make(map[string]bool)
	for _, m := range mirrors {
		unique[m] = true
	}

	var candidates []string
	for m := range unique {
		candidates = append(candidates, m)
	}

	utils.Debug("Probing %d mirrors...", len(candidates))

	valid = make([]string, 0, len(candidates))
	errors = make(map[string]error)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, url := range candidates {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()

			// Short timeout for bulk probing
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			result, err := ProbeServer(probeCtx, target, "", nil)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors[target] = err
				return
			}

			if result.SupportsRange {
				valid = append(valid, target)
			} else {
				errors[target] = fmt.Errorf("does not support ranges")
			}
		}(url)
	}

	wg.Wait()
	utils.Debug("Mirror probing complete: %d valid, %d failed", len(valid), len(errors))
	return valid, errors
}
