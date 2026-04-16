package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/utils"
)

var (
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrDownloadNotFound   = errors.New("download not found")
	ErrNoDestinationPath  = errors.New("download has no destination path")
)

func registerHTTPRoutes(mux *http.ServeMux, port int, defaultOutputDir string, service core.DownloadService) {
	mux.HandleFunc("/health", handleHealth(port))
	mux.HandleFunc("/events", eventsHandler(service))
	mux.HandleFunc("/download", handleDownloadRoute(defaultOutputDir, service))
	mux.HandleFunc("/pause", requireMethod(http.MethodPost, withRequiredID(handlePause(service))))
	mux.HandleFunc("/resume", requireMethod(http.MethodPost, withRequiredID(handleResume(service))))
	mux.HandleFunc("/delete", requireMethods(withRequiredID(handleDelete(service)), http.MethodDelete, http.MethodPost))
	mux.HandleFunc("/list", requireMethod(http.MethodGet, handleList(service)))
	mux.HandleFunc("/history", requireMethod(http.MethodGet, handleHistory(service)))
	mux.HandleFunc("/open-file", requireMethod(http.MethodPost, withRequiredID(handleOpenFile(service))))
	mux.HandleFunc("/open-folder", requireMethod(http.MethodPost, withRequiredID(handleOpenFolder(service))))
	mux.HandleFunc("/update-url", requireMethod(http.MethodPut, withRequiredID(handleUpdateURL(service))))
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
	if settings != nil && settings.General.AllowRemoteOpenActions {
		return nil
	}

	return errors.New("open actions are only allowed from local host")
}

func decodeJSONBody(r *http.Request, dst interface{}) error {
	defer func() {
		_ = r.Body.Close()
	}()
	return json.NewDecoder(r.Body).Decode(dst)
}
