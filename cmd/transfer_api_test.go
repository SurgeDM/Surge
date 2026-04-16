package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/backup"
)

func clearImportSessionStoreForTest(t *testing.T) {
	t.Helper()

	importSessionStore.mu.Lock()
	defer importSessionStore.mu.Unlock()
	for id, session := range importSessionStore.items {
		_ = os.Remove(session.Path)
		delete(importSessionStore.items, id)
	}
}

func createImportBundleForTest(t *testing.T) []byte {
	t.Helper()

	setupXDGEnvIsolation(t)
	if err := initializeGlobalState(); err != nil {
		t.Fatalf("initializeGlobalState failed: %v", err)
	}

	var buf bytes.Buffer
	if _, err := backup.Export(context.Background(), &buf, backup.ExportOptions{}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	return buf.Bytes()
}

func TestTransferPreviewEndpoint_EnforcesUploadSizeLimit(t *testing.T) {
	clearImportSessionStoreForTest(t)

	originalLimit := maxImportPreviewSize
	maxImportPreviewSize = 32
	t.Cleanup(func() {
		maxImportPreviewSize = originalLimit
		clearImportSessionStoreForTest(t)
	})

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", &httpAPITestService{})

	req := httptest.NewRequest(http.MethodPost, "/data/import/preview", strings.NewReader(strings.Repeat("a", 64)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected oversized preview upload to fail, got %d", rec.Code)
	}
}

func TestTransferPreviewEndpoint_GeneratesRandomSessionIDs(t *testing.T) {
	clearImportSessionStoreForTest(t)
	t.Cleanup(func() {
		clearImportSessionStoreForTest(t)
	})

	bundle := createImportBundleForTest(t)

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", &httpAPITestService{})

	var previews [2]backup.ImportPreview
	for i := range previews {
		req := httptest.NewRequest(http.MethodPost, "/data/import/preview", bytes.NewReader(bundle))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d returned %d: %s", i, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &previews[i]); err != nil {
			t.Fatalf("unmarshal preview %d failed: %v", i, err)
		}
		if len(previews[i].SessionID) != 32 {
			t.Fatalf("session id %d length=%d, want 32", i, len(previews[i].SessionID))
		}
	}

	if previews[0].SessionID == previews[1].SessionID {
		t.Fatal("expected preview session IDs to differ")
	}
}
