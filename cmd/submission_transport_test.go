package cmd

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
)

func TestManifestSubmissionUsesConfiguredCertificateAuthority(t *testing.T) {
	previousCA := globalTLSCAFile
	t.Cleanup(func() { globalTLSCAFile = previousCA })
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing API authentication")
		}
		switch r.URL.Path {
		case "/capabilities":
			_ = json.NewEncoder(w).Encode(serverCapabilities{SchemaVersion: 1})
		case "/download":
			_ = json.NewEncoder(w).Encode(submissionReceipt{ID: "accepted-id"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	globalTLSCAFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(globalTLSCAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	command := newAddCommand()
	command.SetContext(context.Background())
	items, err := submitPrepared(command, []DownloadRequest{{URL: "https://example.com/model", SkipApproval: true}}, server.URL, "test-token", "")
	if err != nil || len(items) != 1 || items[0].ID != "accepted-id" || items[0].Error != nil {
		t.Fatalf("custom CA was ignored: %+v %v", items, err)
	}
}
func TestManifestSubmissionRejectsCrossOriginRedirect(t *testing.T) {
	var redirected atomic.Int32
	target := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		_ = json.NewEncoder(w).Encode(serverCapabilities{SchemaVersion: 1})
	}))
	defer target.Close()
	source := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	command := newAddCommand()
	command.SetContext(context.Background())
	if _, err := submitPrepared(command, []DownloadRequest{{URL: "https://example.com/model", SkipApproval: true}}, source.URL, "test-token", ""); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if redirected.Load() != 0 {
		t.Fatal("API credentials sent to a redirected origin")
	}
}
