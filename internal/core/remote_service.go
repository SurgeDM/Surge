package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// RemoteDownloadService implements DownloadService for a remote daemon.
type RemoteDownloadService struct {
	ctx       context.Context
	Client    *http.Client
	SSEClient *http.Client
	cancel    context.CancelFunc
	BaseURL   string
	Token     string
}

// NewRemoteDownloadService creates a new remote service instance.
func NewRemoteDownloadService(baseURL, token string) *RemoteDownloadService {
	ctx, cancel := context.WithCancel(context.Background())
	return &RemoteDownloadService{
		BaseURL:   baseURL,
		Token:     token,
		Client:    &http.Client{Timeout: 30 * time.Second},
		SSEClient: &http.Client{},
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *RemoteDownloadService) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		// Limit error body read to 1KB to prevent DoS
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, types.KB))
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// List returns the status of all active and completed downloads.
func (s *RemoteDownloadService) List(ctx context.Context) ([]types.DownloadStatus, error) {
	resp, err := s.doRequest(ctx, "GET", "/list", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var statuses []types.DownloadStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// History returns completed downloads
func (s *RemoteDownloadService) History(ctx context.Context) ([]types.DownloadEntry, error) {
	resp, err := s.doRequest(ctx, "GET", "/history", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var history []types.DownloadEntry
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		return nil, err
	}
	return history, nil
}

// GetStatus returns a status for a single download by id.
func (s *RemoteDownloadService) GetStatus(ctx context.Context, id string) (*types.DownloadStatus, error) {
	resp, err := s.doRequest(ctx, "GET", "/download?id="+url.QueryEscape(id), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var status types.DownloadStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Add queues a new download.
func (s *RemoteDownloadService) Add(ctx context.Context, rawURL, path, filename string, mirrors []string, headers map[string]string, isExplicitCategory bool, totalSize int64, supportsRange bool) (string, error) {
	req := map[string]interface{}{
		"url":                  rawURL,
		"path":                 path,
		"filename":             filename,
		"mirrors":              mirrors,
		"headers":              headers,
		"skip_approval":        true,
		"is_explicit_category": isExplicitCategory,
		"total_size":           totalSize,
		"supports_range":       supportsRange,
	}

	resp, err := s.doRequest(ctx, "POST", "/download", req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result["id"], nil
}

// AddWithID queues a new download with a caller-provided id.
func (s *RemoteDownloadService) AddWithID(ctx context.Context, rawURL, path, filename string, mirrors []string, headers map[string]string, id string, totalSize int64, supportsRange bool) (string, error) {
	req := map[string]interface{}{
		"url":            rawURL,
		"path":           path,
		"filename":       filename,
		"mirrors":        mirrors,
		"headers":        headers,
		"skip_approval":  true,
		"id":             id,
		"total_size":     totalSize,
		"supports_range": supportsRange,
	}

	resp, err := s.doRequest(ctx, "POST", "/download", req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result["id"], nil
}

// Pause pauses an active download.
func (s *RemoteDownloadService) Pause(ctx context.Context, id string) error {
	resp, err := s.doRequest(ctx, "POST", "/pause?id="+url.QueryEscape(id), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// Resume resumes a paused download.
func (s *RemoteDownloadService) Resume(ctx context.Context, id string) error {
	resp, err := s.doRequest(ctx, "POST", "/resume?id="+url.QueryEscape(id), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// ResumeBatch resumes multiple paused downloads efficiently.
func (s *RemoteDownloadService) ResumeBatch(ctx context.Context, ids []string) []error {
	errs := make([]error, len(ids))
	for i, id := range ids {
		errs[i] = s.Resume(ctx, id)
	}
	return errs
}

// UpdateURL updates the URL of a paused or errored download via the remote API.
func (s *RemoteDownloadService) UpdateURL(ctx context.Context, id, newURL string) error {
	req := map[string]string{
		"url": newURL,
	}
	resp, err := s.doRequest(ctx, "PUT", "/update-url?id="+url.QueryEscape(id), req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// Delete cancels and removes a download.
func (s *RemoteDownloadService) Delete(ctx context.Context, id string) error {
	resp, err := s.doRequest(ctx, "POST", "/delete?id="+url.QueryEscape(id), nil)
	// Some APIs use DELETE method, checking previous implementation in server it supports both POST and DELETE
	// but mostly POST for actions. Let's stick to POST as per server implementation.
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// Shutdown stops the service.
func (s *RemoteDownloadService) Shutdown() error {
	s.cancel()
	return nil
}

// StreamEvents returns a channel that receives real-time download events via SSE.
func (s *RemoteDownloadService) StreamEvents(ctx context.Context) (eventCh <-chan interface{}, cleanupFn func(), err error) {
	ch := make(chan interface{}, 100)
	go s.streamWithReconnect(ctx, ch)
	return ch, func() {}, nil
}

// Publish emits an event into the service's event stream.
// Remote services do not accept client-side event injection.
func (s *RemoteDownloadService) Publish(msg interface{}) error {
	return errors.New("publish not supported for remote service")
}

func (s *RemoteDownloadService) streamWithReconnect(ctx context.Context, ch chan interface{}) {
	defer close(ch)
	backoff := 1 * time.Second
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ctx.Done():
			return
		default:
		}

		err := s.connectSSE(ctx, ch)
		if err == nil {
			return // Clean shutdown (e.g. server closed stream cleanly or context canceled during request)
		}
		// Check context again before sleeping
		select {
		case <-s.ctx.Done():
			return
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			// Continue
		}

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *RemoteDownloadService) connectSSE(ctx context.Context, ch chan interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/events", http.NoBody)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := s.SSEClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to connect to event stream: %s", resp.Status)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		eventType := ""
		var dataLines []string

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimRight(line, "\r\n")

			// Blank line dispatches event
			if line == "" {
				break
			}
			// Comment/heartbeat
			if strings.HasPrefix(line, ":") {
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				continue
			}
		}

		if eventType == "" || len(dataLines) == 0 {
			continue
		}
		jsonData := strings.Join(dataLines, "\n")

		msg, ok, err := events.DecodeSSEMessage(eventType, []byte(jsonData))
		if err != nil {
			utils.Debug("SSE decode error for event=%s payload_bytes=%d: %v", eventType, len(jsonData), err)
			continue
		}
		if !ok {
			continue
		}

		// Non-blocking send
		select {
		case ch <- msg:
		default:
			// Drop message if channel is full to prevent blocking the reader
		}
	}
}
