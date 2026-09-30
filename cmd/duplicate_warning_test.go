package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestDuplicateCheckIncludesPersistedDownloads(t *testing.T) {
	setupIsolatedCmdState(t)
	const duplicateURL = "https://example.com/finished.zip"
	if err := store.AddToMasterList(types.DownloadRecord{
		ID: "finished", URL: duplicateURL, Filename: "finished.zip", Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", nil)
	for _, tc := range []struct {
		url    string
		exists bool
	}{
		{duplicateURL, true},
		{duplicateURL + ",https://mirror.example.com/finished.zip", true},
		{"https://example.com/new.zip," + duplicateURL, false},
		{"https://example.com/new.zip", false},
	} {
		body, _ := json.Marshal(map[string]string{"url": tc.url})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/download/duplicate", bytes.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("duplicate check for %q returned %d: %s", tc.url, rec.Code, rec.Body.String())
		}
		var result struct {
			Exists bool `json:"exists"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Exists != tc.exists {
			t.Errorf("duplicate check for %q: exists = %v, want %v", tc.url, result.Exists, tc.exists)
		}
	}
}

func TestSkipDuplicateWarningPreservesExtensionPrompt(t *testing.T) {
	settings := config.DefaultSettings()
	settings.General.WarnOnDuplicate.Value = true
	settings.Extension.ExtensionPrompt.Value = false
	resolved := &resolvedDownloadRequest{
		settings:    settings,
		isDuplicate: true,
		request:     DownloadRequest{SkipDuplicateWarning: true},
	}
	if shouldRequireDownloadApproval(resolved) {
		t.Fatal("duplicate warning was not suppressed")
	}
	resolved.request.SkipDuplicateWarning = false
	if !shouldRequireDownloadApproval(resolved) {
		t.Fatal("expected duplicate warning when enabled")
	}
	resolved.request.SkipDuplicateWarning = true
	settings.Extension.ExtensionPrompt.Value = true
	if !shouldRequireDownloadApproval(resolved) {
		t.Fatal("general extension prompt was suppressed")
	}
}

func TestSkipDuplicateWarningAutoApprovesInHeadlessMode(t *testing.T) {
	originalProgram := serverProgram
	serverProgram = nil
	t.Cleanup(func() { serverProgram = originalProgram })
	settings := config.DefaultSettings()
	resolved := &resolvedDownloadRequest{
		settings:    settings,
		isDuplicate: true,
		request:     DownloadRequest{SkipDuplicateWarning: true},
	}
	rec := httptest.NewRecorder()
	if maybeRequireDownloadApproval(rec, nil, resolved) {
		t.Fatalf("headless duplicate was rejected: %d %s", rec.Code, rec.Body.String())
	}
}
