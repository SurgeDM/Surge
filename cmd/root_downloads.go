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

func handleDownload(w http.ResponseWriter, r *http.Request, defaultOutputDir string, service core.DownloadService) {
	// GET request to query status
	if r.Method == http.MethodGet {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		if service == nil {
			http.Error(w, "Service unavailable", http.StatusInternalServerError)
			return
		}

		status, err := service.GetStatus(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		writeJSONResponse(w, http.StatusOK, status)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := getSettings()

	var req DownloadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	if strings.Contains(req.Path, "..") || strings.Contains(req.Filename, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "\\") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	utils.Debug("Received download request: URL=%s, Path=%s", req.URL, req.Path)

	if service == nil {
		http.Error(w, "Service unavailable", http.StatusInternalServerError)
		return
	}

	// Prepare output path
	outPath := resolveOutputDir(req.Path, req.RelativeToDefaultDir, defaultOutputDir, settings)

	// Enforce absolute path to ensure resume works even if CWD changes
	outPath = utils.EnsureAbsPath(outPath)

	// Check settings for extension prompt and duplicates
	// Logic modified to distinguish between ACTIVE (corruption risk) and COMPLETED (overwrite safe)
	isDuplicate := false
	isActive := false

	urlForAdd := req.URL
	mirrorsForAdd := req.Mirrors
	if len(mirrorsForAdd) == 0 && strings.Contains(req.URL, ",") {
		urlForAdd, mirrorsForAdd = ParseURLArg(req.URL)
	}

	activeDownloadsFunc := func() map[string]*types.DownloadConfig {
		active := make(map[string]*types.DownloadConfig)
		for _, cfg := range GlobalPool.GetAll() {
			c := cfg // create copy
			active[c.ID] = &c
		}
		return active
	}
	dupResult := processing.CheckForDuplicate(urlForAdd, settings, activeDownloadsFunc)
	if dupResult != nil {
		isDuplicate = dupResult.Exists
		isActive = dupResult.IsActive
	}

	utils.Debug("Download request: URL=%s, SkipApproval=%v, isDuplicate=%v, isActive=%v", urlForAdd, req.SkipApproval, isDuplicate, isActive)

	// EXTENSION VETTING SHORTCUT:
	// If SkipApproval is true, we trust the extension completely.
	// The backend will auto-rename duplicate files, so no need to reject.
	if req.SkipApproval {
		// Trust extension -> Skip all prompting logic, proceed to download
		utils.Debug("Extension request: skipping all prompts, proceeding with download")
	} else {
		// Logic for prompting:
		// 1. If ExtensionPrompt is enabled
		// 2. OR if WarnOnDuplicate is enabled AND it is a duplicate
		shouldPrompt := settings.General.ExtensionPrompt || (settings.General.WarnOnDuplicate && isDuplicate)

		// Only prompt if we have a UI running (serverProgram != nil)
		if shouldPrompt {
			if serverProgram != nil {
				utils.Debug("Requesting TUI confirmation for: %s (Duplicate: %v)", req.URL, isDuplicate)

				// Send request to TUI
				downloadID := uuid.New().String()
				if err := service.Publish(events.DownloadRequestMsg{
					ID:       downloadID,
					URL:      urlForAdd,
					Filename: req.Filename,
					Path:     outPath, // Use the path we resolved (default or requested)
					Mirrors:  mirrorsForAdd,
					Headers:  req.Headers,
				}); err != nil {
					http.Error(w, "Failed to notify TUI: "+err.Error(), http.StatusInternalServerError)
					return
				}

				// Return 202 Accepted to indicate it's pending approval
				writeJSONResponse(w, http.StatusAccepted, map[string]string{
					"status":  "pending_approval",
					"message": "Download request sent to TUI for confirmation",
					"id":      downloadID, // ID might change if user modifies it, but useful for tracking
				})
				return
			}

			// Headless mode check
			writeJSONResponse(w, http.StatusConflict, map[string]string{
				"status":  "error",
				"message": "Download rejected: Duplicate download or approval required (Headless mode)",
			})
			return
		}
	}

	lifecycle, err := lifecycleForLocalService(service)
	if err != nil {
		http.Error(w, "Failed to initialize lifecycle manager: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var newID string
	if lifecycle != nil {
		newID, err = lifecycle.Enqueue(r.Context(), &processing.DownloadRequest{
			URL:                urlForAdd,
			Filename:           req.Filename,
			Path:               outPath,
			Mirrors:            mirrorsForAdd,
			Headers:            req.Headers,
			IsExplicitCategory: req.IsExplicitCategory,
			SkipApproval:       req.SkipApproval,
		})
	} else {
		newID, err = service.Add(urlForAdd, outPath, req.Filename, mirrorsForAdd, req.Headers, req.IsExplicitCategory, 0, false)
	}
	if err != nil {
		http.Error(w, "Failed to add download: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Increment active downloads counter
	atomic.AddInt32(&activeDownloads, 1)

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"status":  "queued",
		"message": "Download queued successfully",
		"id":      newID,
	})
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

	_ = os.MkdirAll(outPath, 0o755)
	return outPath
}
