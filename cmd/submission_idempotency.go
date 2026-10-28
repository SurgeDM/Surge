package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/SurgeDM/Surge/internal/idempotency"
	"github.com/SurgeDM/Surge/internal/store"
)

func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func submissionLedger() *idempotency.Ledger {
	dir := store.StateDirectory()
	if dir == "" {
		return nil
	}
	return idempotency.New(filepath.Join(dir, "idempotency.json"))
}
func validIdempotencyKey(key string) bool {
	if len(key) < 1 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func reserveSubmission(w http.ResponseWriter, r *http.Request, resolved *resolvedDownloadRequest, ledger *idempotency.Ledger) (func(bool) error, bool) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return nil, false
	}
	if ledger == nil {
		http.Error(w, "Durable idempotency unavailable", http.StatusNotImplemented)
		return nil, true
	}
	if !validIdempotencyKey(key) {
		http.Error(w, "Idempotency-Key must contain 1-128 printable ASCII characters", http.StatusBadRequest)
		return nil, true
	}
	if !resolved.request.SkipApproval && shouldRequireDownloadApproval(resolved) {
		http.Error(w, "Idempotent submissions require immediate acceptance", http.StatusConflict)
		return nil, true
	}
	canonical := resolved.request
	canonical.Path = resolved.outPath
	canonical.URL = resolved.urlForAdd
	canonical.Mirrors = resolved.mirrorsForAdd
	canonical.RelativeToDefaultDir = false
	data, err := json.Marshal(canonical)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return nil, true
	}
	scopeKey := digestText(r.Header.Get("Authorization")) + ":" + digestText(key)
	record, owned, err := ledger.Reserve(r.Context(), scopeKey, digestText(string(data)))
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, idempotency.ErrConflict) || errors.Is(err, idempotency.ErrUnknown) {
			status = http.StatusConflict
		}
		if record.ID != "" {
			w.Header().Set("X-Surge-Download-ID", record.ID)
		}
		http.Error(w, "Idempotent submission unavailable; inspect X-Surge-Download-ID before retrying", status)
		return nil, true
	}
	if !owned {
		w.Header().Set("Idempotency-Replayed", "true")
		writeJSONResponse(w, http.StatusOK, map[string]string{"id": record.ID, "status": "queued"})
		return nil, true
	}
	query := r.URL.Query()
	query.Set("id", record.ID)
	r.URL.RawQuery = query.Encode()
	return func(accepted bool) error { return ledger.Finish(scopeKey, accepted) }, false
}
func itemIdempotencyKey(key, ref string, index int) string {
	if key == "" {
		return ""
	}
	// Hash the client namespace and stable reference; inputs never enter headers.
	if ref != "" {
		return digestText(key + "\x00ref:" + ref)
	}
	return digestText(key + "\x00index:" + strconv.Itoa(index))
}
