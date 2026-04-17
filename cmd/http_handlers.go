package cmd

import (
	"net/http"
	"sort"

	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/utils"
)

func handleHealth(port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"port":   port,
		})
	}
}

func handleDownloadRoute(defaultOutputDir string, service core.DownloadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleDownload(w, r, defaultOutputDir, service)
	}
}

func handlePause(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
		if err := service.Pause(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
	}
}

func handleResume(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
		if err := service.Resume(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
	}
}

func handleDelete(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
		if err := service.Delete(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}
}

func handleList(service core.DownloadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses, err := service.List(r.Context())
		if err != nil {
			http.Error(w, "Failed to list downloads: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONResponse(w, http.StatusOK, statuses)
	}
}

func handleHistory(service core.DownloadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		history, err := service.History(r.Context())
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
	}
}

func handleOpenFile(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
		if err := ensureOpenActionRequestAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		destPath, err := resolveDownloadDestPath(r.Context(), service, id)
		if err != nil {
			http.Error(w, err.Error(), statusCodeForResolveDownloadError(err))
			return
		}

		if err := utils.OpenFile(r.Context(), destPath); err != nil {
			http.Error(w, "Failed to open file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "id": id})
	}
}

func handleOpenFolder(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
		if err := ensureOpenActionRequestAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		destPath, err := resolveDownloadDestPath(r.Context(), service, id)
		if err != nil {
			http.Error(w, err.Error(), statusCodeForResolveDownloadError(err))
			return
		}

		if err := utils.OpenContainingFolder(r.Context(), destPath); err != nil {
			http.Error(w, "Failed to open folder: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok", "id": id})
	}
}

func handleUpdateURL(service core.DownloadService) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, id string) {
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

		if err := service.UpdateURL(r.Context(), id, newURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "updated", "id": id, "url": newURL})
	}
}
