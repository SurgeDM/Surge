package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/utils"
)

// DownloadRequest represents a download request from the browser extension
type DownloadRequest struct {
	URL                  string            `json:"url"`
	Filename             string            `json:"filename,omitempty"`
	Path                 string            `json:"path,omitempty"`
	RelativeToDefaultDir bool              `json:"relative_to_default_dir,omitempty"`
	Mirrors              []string          `json:"mirrors,omitempty"`
	SkipApproval         bool              `json:"skip_approval,omitempty"` // Extension validated request, skip TUI prompt
	Headers              map[string]string `json:"headers,omitempty"`       // Custom HTTP headers from browser (cookies, auth, etc.)
	IsExplicitCategory   bool              `json:"is_explicit_category,omitempty"`
}

type resolvedDownloadRequest struct {
	request       DownloadRequest
	settings      *config.Settings
	outPath       string
	urlForAdd     string
	mirrorsForAdd []string
	isDuplicate   bool
	isActive      bool
}

func handleDownload(w http.ResponseWriter, r *http.Request, defaultOutputDir string, service core.DownloadService) {
	if handleDownloadStatusRequest(w, r, service) {
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if service == nil {
		http.Error(w, "Service unavailable", http.StatusInternalServerError)
		return
	}

	resolved, err := resolveDownloadRequest(r, defaultOutputDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if maybeRequireDownloadApproval(w, service, resolved) {
		return
	}

	newID, err := enqueueDownloadRequest(r, service, resolved)
	if err != nil {
		recordPreflightDownloadError(resolved.urlForAdd, resolved.outPath, err)
		publishSystemLog(fmt.Sprintf("Error adding %s: %v", resolved.urlForAdd, err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	atomic.AddInt32(&activeDownloads, 1)
	writeJSONResponse(w, http.StatusOK, map[string]string{
		"status":  "queued",
		"message": "Download queued successfully",
		"id":      newID,
	})
}

func handleDownloadStatusRequest(w http.ResponseWriter, r *http.Request, service core.DownloadService) bool {
	if r.Method != http.MethodGet {
		return false
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing id parameter", http.StatusBadRequest)
		return true
	}

	if service == nil {
		http.Error(w, "Service unavailable", http.StatusInternalServerError)
		return true
	}

	status, err := service.GetStatus(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return true
	}

	writeJSONResponse(w, http.StatusOK, status)
	return true
}

func decodeAndValidateDownloadRequest(r *http.Request) (DownloadRequest, error) {
	var req DownloadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		return req, fmt.Errorf("Invalid JSON: %w", err)
	}
	if req.URL == "" {
		return req, fmt.Errorf("URL is required")
	}
	if strings.Contains(req.Filename, "..") {
		return req, fmt.Errorf("Invalid filename")
	}
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "\\") {
		return req, fmt.Errorf("Invalid filename")
	}
	if strings.Contains(req.Path, "..") {
		return req, fmt.Errorf("Invalid path")
	}
	if req.RelativeToDefaultDir && req.Path != "" {
		if filepath.IsAbs(req.Path) {
			return req, fmt.Errorf("Invalid path")
		}
		cleanPath := filepath.Clean(req.Path)
		if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
			return req, fmt.Errorf("Invalid path")
		}
		req.Path = cleanPath
	}
	return req, nil
}

func resolveDownloadRequest(r *http.Request, defaultOutputDir string) (*resolvedDownloadRequest, error) {
	settings := getSettings()
	req, err := decodeAndValidateDownloadRequest(r)
	if err != nil {
		return nil, err
	}

	utils.Debug("Received download request: URL=%s, Path=%s", req.URL, req.Path)

	outPath := utils.EnsureAbsPath(resolveOutputDir(req.Path, req.RelativeToDefaultDir, defaultOutputDir, settings))
	urlForAdd, mirrorsForAdd := normalizeDownloadTargets(req.URL, req.Mirrors)
	isDuplicate, isActive := resolveDuplicateState(urlForAdd, settings)

	utils.Debug("Download request: URL=%s, SkipApproval=%v, isDuplicate=%v, isActive=%v", urlForAdd, req.SkipApproval, isDuplicate, isActive)

	return &resolvedDownloadRequest{
		request:       req,
		settings:      settings,
		outPath:       outPath,
		urlForAdd:     urlForAdd,
		mirrorsForAdd: mirrorsForAdd,
		isDuplicate:   isDuplicate,
		isActive:      isActive,
	}, nil
}

func normalizeDownloadTargets(url string, mirrors []string) (string, []string) {
	if len(mirrors) == 0 && strings.Contains(url, ",") {
		return ParseURLArg(url)
	}
	return url, mirrors
}

func resolveDuplicateState(urlForAdd string, settings *config.Settings) (bool, bool) {
	activeDownloadsFunc := func() map[string]*types.DownloadConfig {
		active := make(map[string]*types.DownloadConfig)
		for _, cfg := range GlobalPool.GetAll() {
			c := cfg
			active[c.ID] = &c
		}
		return active
	}

	dupResult := processing.CheckForDuplicate(urlForAdd, settings, activeDownloadsFunc)
	if dupResult == nil {
		return false, false
	}
	return dupResult.Exists, dupResult.IsActive
}

func maybeRequireDownloadApproval(w http.ResponseWriter, service core.DownloadService, resolved *resolvedDownloadRequest) bool {
	req := resolved.request

	// EXTENSION VETTING SHORTCUT:
	// If SkipApproval is true, we trust the extension completely.
	// The backend will auto-rename duplicate files, so no need to reject.
	if req.SkipApproval {
		utils.Debug("Extension request: skipping all prompts, proceeding with download")
		return false
	}

	shouldPrompt := resolved.settings.General.ExtensionPrompt || (resolved.settings.General.WarnOnDuplicate && resolved.isDuplicate)
	if !shouldPrompt {
		return false
	}

	if serverProgram != nil {
		utils.Debug("Requesting TUI confirmation for: %s (Duplicate: %v)", req.URL, resolved.isDuplicate)

		downloadID := uuid.New().String()
		if err := service.Publish(events.DownloadRequestMsg{
			ID:       downloadID,
			URL:      resolved.urlForAdd,
			Filename: req.Filename,
			Path:     resolved.outPath,
			Mirrors:  resolved.mirrorsForAdd,
			Headers:  req.Headers,
		}); err != nil {
			recordPreflightDownloadError(resolved.urlForAdd, resolved.outPath, err)
			publishSystemLog(fmt.Sprintf("Error adding %s: %v", resolved.urlForAdd, err))
			http.Error(w, "Failed to notify TUI: "+err.Error(), http.StatusInternalServerError)
			return true
		}

		writeJSONResponse(w, http.StatusAccepted, map[string]string{
			"status":  "pending_approval",
			"message": "Download request sent to TUI for confirmation",
			"id":      downloadID,
		})
		return true
	}

	writeJSONResponse(w, http.StatusConflict, map[string]string{
		"status":  "error",
		"message": "Download rejected: Duplicate download or approval required (Headless mode)",
	})
	return true
}

func enqueueDownloadRequest(r *http.Request, service core.DownloadService, resolved *resolvedDownloadRequest) (string, error) {
	lifecycle, err := lifecycleForLocalService(service)
	if err != nil {
		return "", fmt.Errorf("Failed to initialize lifecycle manager: %w", err)
	}

	req := resolved.request
	if lifecycle != nil {
		return lifecycle.Enqueue(r.Context(), &processing.DownloadRequest{
			URL:                resolved.urlForAdd,
			Filename:           req.Filename,
			Path:               resolved.outPath,
			Mirrors:            resolved.mirrorsForAdd,
			Headers:            req.Headers,
			IsExplicitCategory: req.IsExplicitCategory,
			SkipApproval:       req.SkipApproval,
		})
	}

	return service.Add(resolved.urlForAdd, resolved.outPath, req.Filename, resolved.mirrorsForAdd, req.Headers, req.IsExplicitCategory, 0, false)
}

// processDownloads handles the logic of adding downloads either to local pool or remote server
// Returns the number of successfully added downloads
func processDownloads(urls []string, outputDir string, port int) int {
	successCount := 0

	// If port > 0, we are sending to a remote server
	if port > 0 {
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		token := resolveLocalToken()
		for _, arg := range urls {
			url, mirrors := ParseURLArg(arg)
			if url == "" {
				continue
			}
			err := sendToServer(url, mirrors, outputDir, baseURL, token)
			if err != nil {
				fmt.Printf("Error adding %s: %v\n", url, err)
			} else {
				successCount++
			}
		}
		return successCount
	}

	// Internal add (TUI or Headless mode)
	if GlobalService == nil {
		fmt.Fprintln(os.Stderr, "Error: GlobalService not initialized")
		return 0
	}

	settings := getSettings()

	lifecycle, err := lifecycleForLocalService(GlobalService)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: unable to initialize lifecycle manager:", err)
		return 0
	}

	for _, arg := range urls {
		// Validation
		if arg == "" {
			continue
		}

		url, mirrors := ParseURLArg(arg)
		if url == "" {
			continue
		}

		// Prepare output path
		outPath := resolveOutputDir(outputDir, false, "", settings)
		outPath = utils.EnsureAbsPath(outPath)

		// CLI explicit arg means we do not auto-route when user provided an explicit output path.
		isExplicit := isExplicitOutputPath(outPath, settings.General.DefaultDownloadDir)
		if lifecycle == nil {
			err := fmt.Errorf("lifecycle manager unavailable")
			recordPreflightDownloadError(url, outPath, err)
			publishSystemLog(fmt.Sprintf("Error adding %s: %v", url, err))
			continue
		}

		_, err := lifecycle.Enqueue(currentEnqueueContext(), &processing.DownloadRequest{
			URL:                url,
			Path:               outPath,
			Mirrors:            mirrors,
			IsExplicitCategory: isExplicit,
		})
		if err != nil {
			recordPreflightDownloadError(url, outPath, err)
			publishSystemLog(fmt.Sprintf("Error adding %s: %v", url, err))
			continue
		}
		atomic.AddInt32(&activeDownloads, 1)
		successCount++
	}
	return successCount
}

func resolveOutputDir(reqPath string, relativeToDefaultDir bool, defaultOutputDir string, settings *config.Settings) string {
	outPath := reqPath

	if relativeToDefaultDir && reqPath != "" {
		baseDir := settings.General.DefaultDownloadDir
		if baseDir == "" {
			baseDir = defaultOutputDir
		}
		if baseDir == "" {
			baseDir = "."
		}
		outPath = filepath.Join(baseDir, reqPath)
	} else if outPath == "" {
		if defaultOutputDir != "" {
			outPath = defaultOutputDir
		} else if settings.General.DefaultDownloadDir != "" {
			outPath = settings.General.DefaultDownloadDir
		} else {
			outPath = "."
		}
	}

	return outPath
}
