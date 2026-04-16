package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/backup"
)

// TransferService defines import/export operations for local and remote use.
type TransferService interface {
	Export(ctx context.Context, opts backup.ExportOptions, dst io.Writer) (*backup.Manifest, error)
	PreviewImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportPreview, error)
	ApplyImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportResult, error)
}

type LocalTransferService struct {
	Controller backup.Controller
	Version    string
}

func NewLocalTransferService(controller backup.Controller, version string) *LocalTransferService {
	return &LocalTransferService{
		Controller: controller,
		Version:    strings.TrimSpace(version),
	}
}

func (s *LocalTransferService) Export(ctx context.Context, opts backup.ExportOptions, dst io.Writer) (*backup.Manifest, error) {
	opts.AppVersion = s.Version
	return backup.Export(ctx, dst, opts, s.Controller)
}

func (s *LocalTransferService) PreviewImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportPreview, error) {
	return backup.PreviewImport(ctx, src, opts)
}

func (s *LocalTransferService) ApplyImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportResult, error) {
	return backup.ApplyImport(ctx, src, opts, s.Controller)
}

type RemoteTransferService struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewRemoteTransferService(baseURL, token string) *RemoteTransferService {
	return &RemoteTransferService{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (s *RemoteTransferService) Export(ctx context.Context, opts backup.ExportOptions, dst io.Writer) (*backup.Manifest, error) {
	body, err := json.Marshal(opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/data/export", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("remote export failed: %s - %s", resp.Status, string(b))
	}
	manifestHeader := strings.TrimSpace(resp.Header.Get("X-Surge-Manifest"))
	var manifest backup.Manifest
	if manifestHeader != "" {
		if decoded, err := url.QueryUnescape(manifestHeader); err == nil {
			_ = json.Unmarshal([]byte(decoded), &manifest)
		}
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion == 0 {
		return &backup.Manifest{}, nil
	}
	return &manifest, nil
}

func (s *RemoteTransferService) PreviewImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportPreview, error) {
	endpoint := s.BaseURL + "/data/import/preview"
	query := url.Values{}
	if strings.TrimSpace(opts.RootDir) != "" {
		query.Set("root_dir", opts.RootDir)
	}
	if opts.Replace {
		query.Set("replace", "true")
	}
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, src)
	if err != nil {
		return nil, err
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("remote preview failed: %s - %s", resp.Status, string(b))
	}

	var preview backup.ImportPreview
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		return nil, err
	}
	return &preview, nil
}

func (s *RemoteTransferService) ApplyImport(ctx context.Context, src io.Reader, opts backup.ImportOptions) (*backup.ImportResult, error) {
	payload := map[string]interface{}{
		"session_id": opts.SessionID,
		"root_dir":   opts.RootDir,
		"replace":    opts.Replace,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/data/import/apply", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("remote apply failed: %s - %s", resp.Status, string(b))
	}
	var result backup.ImportResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	_ = src
	return &result, nil
}
