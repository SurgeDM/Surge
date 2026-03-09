package processing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
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

// ProbeResult contains all metadata from server probe
type ProbeResult struct {
	FileSize      int64
	SupportsRange bool
	Filename      string
	ContentType   string
}

// ProbeServer uses a tiny ranged GET so callers can decide filename handling
// and resumability before handing work to the downloader.
func ProbeServer(ctx context.Context, rawurl string, filenameHint string, headers map[string]string) (*ProbeResult, error) {
	utils.Debug("Probing server: %s", rawurl)

	var resp *http.Response
	var err error

	client := getProbeClient()

	for attempt := range 3 {
		if ctx.Err() != nil {
			if err == nil {
				err = fmt.Errorf("probe request aborted: %w", ctx.Err())
			}
			break
		}

		if attempt > 0 {
			select {
			case <-ctx.Done():
				err = fmt.Errorf("probe request aborted during retry: %w", ctx.Err())
			case <-time.After(1 * time.Second):
			}
			if ctx.Err() != nil {
				break
			}
			utils.Debug("Retrying probe... attempt %d", attempt+1)
		}

		probeCtx, cancel := context.WithTimeout(ctx, types.ProbeTimeout)

		req, reqErr := newProbeRequest(probeCtx, rawurl, headers, true)
		if reqErr != nil {
			cancel()
			err = fmt.Errorf("failed to create probe request: %w", reqErr)
			break
		}

		resp, err = client.Do(req)

		// Some origins reject ranged probes outright; a second request without Range
		// lets us still discover filename and size for sequential downloads.
		if err == nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusMethodNotAllowed) {
			utils.Debug("Probe got %d, retrying without Range header", resp.StatusCode)
			_ = resp.Body.Close() // Close previous response

			reqNoRange, reqNoRangeErr := newProbeRequest(probeCtx, rawurl, headers, false)
			if reqNoRangeErr != nil {
				cancel()
				err = fmt.Errorf("failed to create probe request without range: %w", reqNoRangeErr)
				break
			}

			resp, err = client.Do(reqNoRange)
		}

		cancel()

		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("probe request failed after retries: %w", err)
	}

	defer func() {
		// Only drain a small amount of data (32KB) to allow connection reuse for small responses (e.g., 206 Partial Content).
		// For large responses (e.g., 200 OK), reading the whole file into discard takes too long.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*types.KB))
		_ = resp.Body.Close()
	}()

	utils.Debug("Probe response status: %d", resp.StatusCode)

	result := &ProbeResult{}

	// Only a 206 response proves resume-safe range support; a 200 means the
	// downloader must fall back to a single sequential stream.
	switch resp.StatusCode {
	case http.StatusPartialContent: // 206
		result.SupportsRange = true
		contentRange := resp.Header.Get("Content-Range")
		utils.Debug("Content-Range header: %s", contentRange)
		if contentRange != "" {
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

	name, _, err := utils.DetermineFilename(rawurl, resp, false)
	if err != nil {
		utils.Debug("Error determining filename: %v", err)
		name = "download.bin"
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

func newProbeRequest(ctx context.Context, rawurl string, headers map[string]string, includeRange bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, err
	}
	applyProbeHeaders(req, headers, includeRange)
	return req, nil
}

func applyProbeHeaders(req *http.Request, headers map[string]string, includeRange bool) {
	if req == nil {
		return
	}

	for key, val := range headers {
		if strings.EqualFold(key, "Range") {
			continue
		}
		req.Header.Set(key, val)
	}

	if includeRange {
		req.Header.Set("Range", "bytes=0-0")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
}

func getProbeClient() *http.Client {
	probeClientOnce.Do(func() {
		// Keep one shared client for connection reuse; individual probe contexts own
		// the deadline so shutdown cancellation is not masked by client-wide timeouts.
		probeClient = &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if len(via) > 0 {
					copyProbeRedirectHeaders(req, via[0])
				}
				return nil
			},
		}
	})

	return probeClient
}

func copyProbeRedirectHeaders(dst, src *http.Request) {
	if dst == nil || src == nil {
		return
	}

	if sameProbeRedirectOrigin(dst.URL, src.URL) {
		for key, vals := range src.Header {
			dst.Header[key] = append([]string(nil), vals...)
		}
		return
	}

	for key := range dst.Header {
		delete(dst.Header, key)
	}

	for _, key := range []string{"Range", "User-Agent"} {
		if vals := src.Header.Values(key); len(vals) > 0 {
			dst.Header[key] = append([]string(nil), vals...)
		}
	}
}

func sameProbeRedirectOrigin(a, b *neturl.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// ProbeMirrors concurrently checks a list of mirrors and returns valid ones and per-mirror failures.
func ProbeMirrors(ctx context.Context, mirrors []string) (valid []string, errs map[string]error) {
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
	errs = make(map[string]error)
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
				errs[target] = err
				return
			}

			if result.SupportsRange {
				valid = append(valid, target)
			} else {
				errs[target] = fmt.Errorf("does not support ranges")
			}
		}(url)
	}

	wg.Wait()
	utils.Debug("Mirror probing complete: %d valid, %d failed", len(valid), len(errs))
	return valid, errs
}
