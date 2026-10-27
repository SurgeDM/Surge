package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func seedListingHistory(t *testing.T) string {
	t.Helper()
	id := "deadbeef-1234-1234-1234-1234567890ab"
	if err := store.AddToMasterList(types.DownloadRecord{ID: id, Filename: "saved.bin", Status: "downloading", Downloaded: 100, TotalSize: 200, AvgSpeed: 12345}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLiveEmptyListAndIDResolutionAreAuthoritative(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	id := seedListingHistory(t)
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			_, _ = io.WriteString(w, "[]")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	snapshot, err := readDownloadSnapshot(context.Background(), server.URL, "", false)
	if err != nil || snapshot.source != "live" || len(snapshot.downloads) != 0 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	port := server.Listener.Addr().(*net.TCPAddr).Port
	saveActivePort(port)
	globalToken = "test-token"
	resolved, err := resolveDownloadID("deadbeef")
	if err != nil || resolved == id {
		t.Fatalf("history ID incorrectly resolved as live: %q, %v", resolved, err)
	}
	if _, err := readDownloadDetails(context.Background(), id, server.URL, "", false); clioutput.ExitCode(err) != 4 {
		t.Fatalf("live not-found hidden by history: %v", err)
	}
}

func TestHistoryFallbackPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		history bool
	}{
		{"unavailable", 503, "unavailable", true}, {"unauthorized", 401, "secret", false}, {"forbidden", 403, "secret", false},
		{"internal error", 500, "secret", false}, {"invalid JSON", 200, "{", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupIsolatedCmdState(t)
			resetCommandConnectionState(t)
			seedListingHistory(t)
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			snapshot, err := readDownloadSnapshot(context.Background(), server.URL, "", false)
			if !tc.history {
				if err == nil {
					t.Fatal("live failure was hidden")
				}
				return
			}
			if err != nil || snapshot.source != "history" || snapshot.warning == "" || len(snapshot.downloads) != 1 {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
			status := snapshot.downloads[0]
			if status.Speed != 0 || status.AvgSpeed != 12345 || status.ETA != 0 || status.Connections != 0 {
				t.Fatalf("stale live metrics: %+v", status)
			}
			result := snapshot.result()
			if !result.Stale || result.Items[0].SpeedBytesPerSecond != nil {
				t.Fatalf("result=%+v", result)
			}
			if _, err := readDownloadSnapshot(context.Background(), server.URL, "", true); err == nil {
				t.Fatal("explicit remote target fell back locally")
			}
		})
	}
}

func TestListFreshnessMetadataAndLegacyShape(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		name := "legacy"
		if versioned {
			name = "v1"
		}
		t.Run(name, func(t *testing.T) {
			root := newOutputTestRoot(t)
			seedListingHistory(t)
			args := []string{"ls", "--json"}
			if versioned {
				args = []string{"ls", "--output-version=1"}
			}
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, args, &out, &diagnostic); code != 0 {
				t.Fatalf("exit=%d", code)
			}
			if versioned {
				var result listingResult
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Source != "history" || !result.Stale || len(result.Items) != 1 || result.Items[0].SpeedBytesPerSecond != nil || diagnostic.Len() != 0 {
					t.Fatalf("result=%+v stderr=%q", result, diagnostic.String())
				}
			} else {
				var items []downloadInfo
				if err := json.Unmarshal(out.Bytes(), &items); err != nil {
					t.Fatal(err)
				}
				if len(items) != 1 || items[0].Speed != 0 || !strings.Contains(diagnostic.String(), "saved history") {
					t.Fatalf("items=%+v stderr=%q", items, diagnostic.String())
				}
			}
		})
	}
}

func TestListingCancellationReachesHTTPServer(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverCancelled := make(chan struct{})
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
		close(serverCancelled)
	}))
	defer server.Close()
	_, err := readDownloadSnapshot(ctx, server.URL, "", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	select {
	case <-serverCancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP request was not cancelled")
	}
}

func TestListingClearsInactiveMetricsButPreservesAverage(t *testing.T) {
	snapshot := downloadSnapshot{source: "live", downloads: []types.DownloadStatus{withoutInactiveRates(types.DownloadStatus{Status: "completed", Speed: 120, AvgSpeed: 120, ETA: 1, Connections: 8})}}
	status := snapshot.downloads[0]
	if status.Speed != 0 || status.ETA != 0 || status.Connections != 0 || status.AvgSpeed != 120 {
		t.Fatalf("status=%+v", status)
	}
	var out bytes.Buffer
	if err := printDownloadDetail(&out, status, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Speed:") || !strings.Contains(out.String(), "Average:") {
		t.Fatalf("misleading detail: %s", out.String())
	}
}

func TestFullDownloadIDDoesNotFetchEntireQueue(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	const id = "deadbeef-1234-1234-1234-1234567890ab"
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download" || r.URL.Query().Get("id") != id {
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(types.DownloadStatus{ID: id, Status: "downloading", Speed: 42})
	}))
	defer server.Close()
	snapshot, err := readDownloadDetails(context.Background(), id, server.URL, "", true)
	if err != nil || snapshot.source != "live" || len(snapshot.downloads) != 1 || snapshot.downloads[0].Speed != 42 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
}
