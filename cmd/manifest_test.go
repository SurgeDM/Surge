package cmd

import (
	"context"
	"encoding/json"
	"github.com/SurgeDM/Surge/internal/testutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidationBeforeSubmission(t *testing.T) {
	cases := []struct {
		name, input        string
		partial, wantError bool
	}{
		{"valid", `{"schema_version":1,"items":[{"client_ref":"model","url":"https://example.com/model","sha256":"` + strings.Repeat("a", 64) + `"}]}`, false, false},
		{"invalid", `{"schema_version":1,"items":[{"client_ref":"one","url":"https://example.com/a"},{"client_ref":"two","url":"file:///secret"}]}`, false, true},
		{"partial", `{"schema_version":1,"items":[{"client_ref":"one","url":"https://example.com/a"},{"client_ref":"two","url":"file:///secret"}]}`, true, false},
		{"duplicate", `{"schema_version":1,"items":[{"client_ref":"same","url":"https://example.com/a"},{"client_ref":"same","url":"https://example.com/b"}]}`, true, true},
		{"unknown", `{"schema_version":1,"credentials":"secret","items":[]}`, false, true},
		{"trailing", `{"schema_version":1,"items":[]} {}`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newFetchCommand()
			cmd.SetIn(strings.NewReader(tc.input))
			requests, err := readManifest(cmd, "-", tc.partial)
			if (err != nil) != tc.wantError {
				t.Fatalf("requests=%+v err=%v", requests, err)
			}
			if tc.name == "partial" && (requests[1].inputFailure == nil || requests[0].inputFailure != nil) {
				t.Fatal("partial correlation lost")
			}
		})
	}
	cmd := newFetchCommand()
	cmd.SetIn(strings.NewReader(strings.Repeat("x", maxManifestBytes+1)))
	if _, err := readManifest(cmd, "-", false); err == nil {
		t.Fatal("unbounded manifest")
	}
}
func TestManifestKeyUsesStableReferences(t *testing.T) {
	if itemIdempotencyKey("key", "a", 1) == itemIdempotencyKey("key", "b", 1) {
		t.Fatal("different references share key")
	}
}

func TestPartialManifestSessionContainsOnlyValidJobs(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	source := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "3")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("abc"))
		}
	}))
	defer source.Close()
	directory := t.TempDir()
	path := filepath.Join(directory, "manifest.json")
	manifest := downloadManifest{SchemaVersion: 1, Items: []manifestItem{{ClientRef: "invalid", URL: "file:///secret"}, {ClientRef: "valid", URL: source.URL, Output: directory, Filename: "model.bin"}}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	code, result, _ := executeFetchTest(t, context.Background(), "--manifest", path, "--allow-partial")
	if code != 3 || len(result.Items) != 2 || result.Items[0].Error == nil || result.Items[1].Artifact == nil {
		t.Fatalf("partial manifest lost outcomes: %d %+v", code, result)
	}
	code, resumed, _ := executeFetchTest(t, context.Background(), "--resume", result.SessionID)
	if code != 0 || len(resumed.Items) != 1 || resumed.Items[0].InputIndex != 2 || resumed.Items[0].ClientRef != "valid" {
		t.Fatalf("invalid input entered saved session: %d %+v", code, resumed)
	}
}
