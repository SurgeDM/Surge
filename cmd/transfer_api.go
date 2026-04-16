package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/backup"
	"github.com/SurgeDM/Surge/internal/core"
)

var maxImportPreviewSize int64 = 512 * 1024 * 1024

type stagedImportSession struct {
	Path      string
	CreatedAt time.Time
}

var importSessionStore = struct {
	mu    sync.Mutex
	items map[string]stagedImportSession
}{
	items: make(map[string]stagedImportSession),
}

func cleanupImportSessions() {
	cutoff := time.Now().Add(-1 * time.Hour)
	importSessionStore.mu.Lock()
	defer importSessionStore.mu.Unlock()
	for id, session := range importSessionStore.items {
		if session.CreatedAt.After(cutoff) {
			continue
		}
		_ = os.Remove(session.Path)
		delete(importSessionStore.items, id)
	}
}

func newImportSessionID() (string, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return hex.EncodeToString(token), nil
}

func registerTransferRoutes(mux *http.ServeMux, service core.DownloadService) {
	mux.HandleFunc("/data/export", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		cleanupImportSessions()
		var opts backup.ExportOptions
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil && err != io.EOF {
				http.Error(w, "invalid export request", http.StatusBadRequest)
				return
			}
		}

		transfer := core.NewLocalTransferService(service, Version)
		tmpFile, err := os.CreateTemp("", "surge-export-*.zip")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}()

		manifest, err := transfer.Export(r.Context(), opts, tmpFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tmpFile.Seek(0, 0); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manifestBytes, _ := json.Marshal(manifest)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\"surge-export.surge-export\"")
		w.Header().Set("X-Surge-Manifest", url.QueryEscape(string(manifestBytes)))
		if _, err := io.Copy(w, tmpFile); err != nil {
			return
		}
	}))

	mux.HandleFunc("/data/import/preview", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		cleanupImportSessions()
		opts := backup.ImportOptions{
			RootDir: strings.TrimSpace(r.URL.Query().Get("root_dir")),
			Replace: strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("replace")), "true"),
		}
		tmpFile, err := os.CreateTemp("", "surge-import-preview-*.zip")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = tmpFile.Close() }()
		r.Body = http.MaxBytesReader(w, r.Body, maxImportPreviewSize)
		if _, err := io.Copy(tmpFile, r.Body); err != nil {
			_ = os.Remove(tmpFile.Name())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := tmpFile.Seek(0, 0); err != nil {
			_ = os.Remove(tmpFile.Name())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		transfer := core.NewLocalTransferService(service, Version)
		preview, err := transfer.PreviewImport(context.Background(), tmpFile, opts)
		if err != nil {
			_ = os.Remove(tmpFile.Name())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sessionID, err := newImportSessionID()
		if err != nil {
			_ = os.Remove(tmpFile.Name())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		importSessionStore.mu.Lock()
		importSessionStore.items[sessionID] = stagedImportSession{
			Path:      tmpFile.Name(),
			CreatedAt: time.Now(),
		}
		importSessionStore.mu.Unlock()

		preview.SessionID = sessionID
		writeJSONResponse(w, http.StatusOK, preview)
	}))

	mux.HandleFunc("/data/import/apply", requireMethod(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		cleanupImportSessions()
		var req struct {
			SessionID string `json:"session_id"`
			RootDir   string `json:"root_dir"`
			Replace   bool   `json:"replace"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.SessionID) == "" {
			http.Error(w, "missing session_id", http.StatusBadRequest)
			return
		}

		importSessionStore.mu.Lock()
		session, ok := importSessionStore.items[req.SessionID]
		if ok {
			delete(importSessionStore.items, req.SessionID)
		}
		importSessionStore.mu.Unlock()
		if !ok {
			http.Error(w, "import session not found", http.StatusNotFound)
			return
		}
		defer func() { _ = os.Remove(session.Path) }()

		file, err := os.Open(session.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = file.Close() }()

		transfer := core.NewLocalTransferService(service, Version)
		result, err := transfer.ApplyImport(r.Context(), file, backup.ImportOptions{
			RootDir: req.RootDir,
			Replace: req.Replace,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, result)
	}))
}
