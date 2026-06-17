package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

var (
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrDownloadNotFound   = errors.New("download not found")
	ErrNoDestinationPath  = errors.New("download has no destination path")
)

type rateLimitSettingsService interface {
	SetGlobalRateLimit(rate int64) error
	SetDefaultRateLimit(rate int64) error
}

func registerHTTPRoutes(mux *http.ServeMux, port int, defaultOutputDir string, service core.DownloadService) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"port":   port,
		})
	})

	mux.HandleFunc("/events", eventsHandler(service))

	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		handleDownload(w, r, defaultOutputDir, service)
	})

	mux.HandleFunc("/download/batch", func(w http.ResponseWriter, r *http.Request) {
		handleBatchDownload(w, r, defaultOutputDir, service)
	})

	mux.HandleFunc("/pause", requireMethod(http.MethodPost, withRequiredID(func(w http.ResponseWriter, _ *http.Request, id string) {
		if err := service.Pause(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
	})))

	mux.HandleFunc("/resume", requireMethod(http.MethodPost, withRequiredID(func(w http.ResponseWriter, _ *http.Request, id string) {
		if err := service.Resume(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
	})))

	mux.HandleFunc("/delete", requireMethods(withRequiredID(func(w http.ResponseWriter, _ *http.Request, id string) {
		if err := service.Delete(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}), http.MethodDelete, http.MethodPost))

	mux.HandleFunc("/purge", requireMethods(withRequiredID(func(w http.ResponseWriter, _ *http.Request, id string) {
		if err := service.Purge(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "purged", "id": id})
	}), http.MethodDelete, http.MethodPost))

	mux.HandleFunc("/list", requireMethod(http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
		statuses, err := service.List()
		if err != nil {
			http.Error(w, "Failed to list downloads: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, statuses)
	}))

	mux.HandleFunc("/history", requireMethod(http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
		history, err := service.History()
		if err != nil {
			http.Error(w, "Failed to retrieve history: "+err.Error(), http.StatusInternalServerError)
			return
		}
		sort.Slice(history, func(left, right int) bool {
			if history[left].CompletedAt == history[right].CompletedAt {
				return history[left].ID > history[right].ID
			}
			return history[left].CompletedAt > history[right].CompletedAt
		})
		writeJSONResponse(w, http.StatusOK, history)
	}))

	mux.HandleFunc("/open-file", requireMethod(http.MethodPost, withRequiredID(func(w http.ResponseWriter, r *http.Request, id string) {
		if err := ensureOpenActionRequestAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		destPath, err := resolveDownloadDestPath(service, id)
		if err != nil {
			http.Error(w, err.Error(), statusCodeForResolveDownloadError(err))
			return
		}

		if err := utils.OpenFile(destPath); err != nil {
			http.Error(w, "Failed to open file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "id": id})
	})))

	mux.HandleFunc("/open-folder", requireMethod(http.MethodPost, withRequiredID(func(w http.ResponseWriter, r *http.Request, id string) {
		if err := ensureOpenActionRequestAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		destPath, err := resolveDownloadDestPath(service, id)
		if err != nil {
			http.Error(w, err.Error(), statusCodeForResolveDownloadError(err))
			return
		}

		if err := utils.OpenContainingFolder(destPath); err != nil {
			http.Error(w, "Failed to open folder: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "id": id})
	})))

	mux.HandleFunc("/update-url", requireMethod(http.MethodPut, withRequiredID(func(w http.ResponseWriter, r *http.Request, id string) {
		var req map[string]string
		if err := decodeJSONBody(r, &req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		newURL := req["url"]
		if newURL == "" {
			http.Error(w, "Missing url parameter in body", http.StatusBadRequest)
			return
		}

		if err := service.UpdateURL(id, newURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "updated", "id": id, "url": newURL})
	})))

	mux.HandleFunc("/clear-completed", requireMethod(http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		count, err := service.ClearCompleted()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]int64{"deleted": count})
	}))

	mux.HandleFunc("/clear-failed", requireMethod(http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		count, err := service.ClearFailed()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]int64{"deleted": count})
	}))
	mux.HandleFunc("/rate-limit", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}
		if isRateLimitInheritRequest(r) {
			if err := service.ClearRateLimit(id); err != nil {
				http.Error(w, err.Error(), statusCodeForRateLimitError(err))
				return
			}
			writeJSONResponse(w, http.StatusOK, map[string]string{"status": "rate_limit_inherited", "id": id})
			return
		}
		rate, rateStr, ok := parseRateLimitQuery(w, r)
		if !ok {
			return
		}

		if err := service.SetRateLimit(id, rate); err != nil {
			http.Error(w, err.Error(), statusCodeForRateLimitError(err))
			return
		}
		status := "rate_limited"
		if rate == 0 {
			status = "rate_unlimited"
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": status, "id": id, "rate": rateStr})
	}))

	mux.HandleFunc("/rate-limit/global", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		limiter, ok := service.(rateLimitSettingsService)
		if !ok {
			http.Error(w, "Service does not support global rate limits", http.StatusNotImplemented)
			return
		}
		rate, rateStr, ok := parseRateLimitQuery(w, r)
		if !ok {
			return
		}
		if err := limiter.SetGlobalRateLimit(rate); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		status := "global_rate_limited"
		if rate == 0 {
			status = "global_rate_unlimited"
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": status, "rate": rateStr})
	}))

	mux.HandleFunc("/rate-limit/default", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		limiter, ok := service.(rateLimitSettingsService)
		if !ok {
			http.Error(w, "Service does not support default rate limits", http.StatusNotImplemented)
			return
		}
		rate, rateStr, ok := parseRateLimitQuery(w, r)
		if !ok {
			return
		}
		if err := limiter.SetDefaultRateLimit(rate); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		status := "default_rate_limited"
		if rate == 0 {
			status = "default_rate_unlimited"
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": status, "rate": rateStr})
	}))
}

func statusCodeForRateLimitError(err error) int {
	if errors.Is(err, types.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func isRateLimitInheritRequest(r *http.Request) bool {
	query := r.URL.Query()
	for _, inherit := range query["inherit"] {
		normalized := strings.ToLower(strings.TrimSpace(inherit))
		if normalized == "true" || normalized == "1" || normalized == "yes" || utils.IsRateLimitInherit(inherit) {
			return true
		}
	}

	return utils.IsRateLimitInherit(query.Get("rate"))
}

func parseRateLimitQuery(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	rateStr := r.URL.Query().Get("rate")
	if rateStr == "" {
		http.Error(w, "Missing rate parameter", http.StatusBadRequest)
		return 0, "", false
	}
	rate, err := strconv.ParseInt(rateStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid rate parameter (expected: integer bytes/sec, e.g. 10485760 for 10 MB/s)", http.StatusBadRequest)
		return 0, "", false
	}
	if rate < 0 {
		http.Error(w, "Rate parameter must be non-negative", http.StatusBadRequest)
		return 0, "", false
	}
	return rate, rateStr, true
}

func eventsHandler(service core.DownloadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		stream, cleanup, err := service.StreamEvents(r.Context())
		if err != nil {
			http.Error(w, "Failed to subscribe to events", http.StatusInternalServerError)
			return
		}
		defer cleanup()

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		done := r.Context().Done()
		for {
			select {
			case <-done:
				return
			case msg, ok := <-stream:
				if !ok {
					return
				}

				frames, err := events.EncodeSSEMessages(msg)
				if err != nil {
					utils.Debug("Error encoding SSE event: %v", err)
					continue
				}
				if len(frames) == 0 {
					continue
				}

				for _, frame := range frames {
					_, _ = fmt.Fprintf(w, "event: %s\n", frame.Event)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", frame.Data)
				}
				flusher.Flush()
			}
		}
	}
}

func requireMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return requireMethods(next, method)
}

func requireMethods(next http.HandlerFunc, methods ...string) http.HandlerFunc {
	allowed := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		allowed[method] = struct{}{}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := allowed[r.Method]; !ok {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func withRequiredID(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}
		next(w, r, id)
	}
}

func writeJSONResponse(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		utils.Debug("Failed to encode response: %v", err)
	}
}

func resolveDownloadDestPath(service core.DownloadService, id string) (string, error) {
	if service == nil {
		return "", ErrServiceUnavailable
	}

	status, err := service.GetStatus(id)
	if err == nil && status != nil {
		if destPath := filepath.Clean(status.DestPath); destPath != "" && destPath != "." {
			return destPath, nil
		}
	}

	history, err := service.History()
	if err != nil {
		return "", fmt.Errorf("failed to read history: %w", err)
	}

	for _, entry := range history {
		if entry.ID != id {
			continue
		}
		destPath := filepath.Clean(entry.DestPath)
		if destPath == "" || destPath == "." {
			return "", fmt.Errorf("%w: %s", ErrNoDestinationPath, id)
		}
		return destPath, nil
	}

	return "", fmt.Errorf("%w: %s", ErrDownloadNotFound, id)
}

func statusCodeForResolveDownloadError(err error) int {
	switch {
	case errors.Is(err, ErrDownloadNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrServiceUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func ensureOpenActionRequestAllowed(r *http.Request) error {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		xri := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if xff == "" && xri == "" {
			return nil
		}
	}

	settings := getSettings()
	if settings != nil && config.Resolve[bool](settings.General.AllowRemoteOpenActions) {
		return nil
	}

	return fmt.Errorf("open actions are only allowed from local host")
}

func decodeJSONBody(r *http.Request, dst interface{}) error {
	defer func() {
		_ = r.Body.Close()
	}()
	return json.NewDecoder(r.Body).Decode(dst)
}
