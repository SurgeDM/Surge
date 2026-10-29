package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestWaitReconcilesWithoutEventsAndKeepsCompletedReceipts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := make(map[string]int)
		lookup := func(_ context.Context, id string) (*types.DownloadStatus, error) {
			calls[id]++
			if id == "gone" {
				return nil, clioutput.NewError(clioutput.NotFound, "Job removed.", nil)
			}
			status := "completed"
			if id == "delayed" && calls[id] == 1 {
				status = "finalizing"
			}
			return &types.DownloadStatus{ID: id, Status: status, Downloaded: 9007199254740993, TotalSize: 9007199254740993, DestPath: "/remote/model.bin", Error: "signed-url-secret"}, nil
		}
		items, err := observeDownloads(context.Background(), []string{"ready", "gone", "delayed"}, lookup, time.Second, "remote", "https://daemon.example")
		if clioutput.ExitCode(err) != 3 || len(items) != 3 || calls["ready"] != 1 || calls["gone"] != 1 || calls["delayed"] != 2 {
			t.Fatalf("items=%+v err=%v calls=%v", items, err, calls)
		}
		if artifact := items[2].Artifact; artifact == nil || artifact.Location != "remote" || artifact.Server != "https://daemon.example" || artifact.SizeBytes != "9007199254740993" || artifact.Verification != "not_requested" {
			t.Fatalf("artifact=%+v", artifact)
		}
	})
}

func TestWaitObservationDeadlineAndCancellationLeavePartialReceipts(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupt), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
				defer cancel()
				lookup := func(ctx context.Context, id string) (*types.DownloadStatus, error) {
					if id == "ready" {
						return &types.DownloadStatus{ID: id, Status: "completed", DestPath: "/artifact", Downloaded: 7}, nil
					}
					if interrupt {
						cancel()
					}
					return &types.DownloadStatus{ID: id, Status: "downloading", Downloaded: 2}, nil
				}
				items, err := observeDownloads(ctx, []string{"ready", "active"}, lookup, time.Second, "local", "")
				want := 8
				if interrupt {
					want = 130
				}
				if clioutput.ExitCode(err) != want || items[0].Artifact == nil || items[0].Error != nil || items[1].Error == nil || !strings.Contains(err.Error(), "left running") {
					t.Fatalf("items=%+v err=%v", items, err)
				}
			})
		})
	}
}

func TestWaitReadRetriesAreBoundedAndBackOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		start := time.Now()
		lookup := func(context.Context, string) (*types.DownloadStatus, error) {
			calls++
			return nil, clioutput.NewError(clioutput.Unavailable, "Temporary outage.", nil)
		}
		items, err := observeDownloads(context.Background(), []string{"job"}, lookup, time.Second, "local", "")
		if calls != 6 || time.Since(start) != 30*time.Second || clioutput.ExitCode(err) != 6 || items[0].Error == nil {
			t.Fatalf("calls=%d elapsed=%s items=%+v err=%v", calls, time.Since(start), items, err)
		}
	})
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		lookup := func(context.Context, string) (*types.DownloadStatus, error) {
			calls++
			if calls < 3 {
				return nil, clioutput.NewError(clioutput.Unavailable, "Temporary outage.", nil)
			}
			return &types.DownloadStatus{ID: "job", Status: "completed", DestPath: "/artifact"}, nil
		}
		items, err := observeDownloads(context.Background(), []string{"job"}, lookup, time.Second, "local", "")
		if err != nil || calls != 3 || items[0].Artifact == nil {
			t.Fatalf("calls=%d items=%+v err=%v", calls, items, err)
		}
	})
}

func TestWaitJSONUsesFullIDsAndOneResult(t *testing.T) {
	root := newOutputTestRoot(t)
	root.AddCommand(newWaitCommand())
	id := "12345678-1234-1234-1234-123456789012"
	var gets atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)
		if r.URL.Path != "/download/result" || r.Method != http.MethodGet || r.URL.Query().Get("id") != id || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("X-Surge-Result-Version", "1")
		_ = json.NewEncoder(w).Encode(types.DownloadStatus{ID: id, Status: "completed", DestPath: "/remote/artifact", Downloaded: 42, TotalSize: 42, URL: "https://secret.example/?secret", Error: "secret"})
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	var out, diagnostic bytes.Buffer
	code := runCommand(root, []string{"wait", "--json", id, id}, &out, &diagnostic)
	var result clioutput.Result[waitItem]
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 0 || diagnostic.Len() != 0 || gets.Load() != 1 || !result.OK || result.Operation != "wait" || len(result.Items) != 1 || strings.Contains(out.String(), "secret") {
		t.Fatalf("exit=%d gets=%d stdout=%s stderr=%s", code, gets.Load(), &out, &diagnostic)
	}
	if result.Items[0].Artifact.Location != "remote" {
		t.Fatalf("remote artifact mislabeled: %+v", result)
	}
}

func TestWaitRequiresFinalizedResultCapability(t *testing.T) {
	for _, tc := range []struct {
		header string
		status int
		body   string
		want   clioutput.Code
	}{
		{"", 404, "not found", clioutput.Conflict}, {"", 200, `{}`, clioutput.Conflict},
		{"", 401, "secret", clioutput.Unauthorized},
		{"", 408, "secret", clioutput.Timeout},
		{"", 429, "secret", clioutput.Unavailable},
		{"1", 200, `{"id":"other","status":"completed","dest_path":"/wrong"}`, clioutput.OperationFailed},
		{"1", 200, `{"id":"job","status":"completed"}`, clioutput.OperationFailed},
		{"1", 404, "secret", clioutput.NotFound},
	} {
		t.Run(fmt.Sprint(tc.status, tc.header, tc.body), func(t *testing.T) {
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Surge-Result-Version", tc.header)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := server.Client()
			defer client.CloseIdleConnections()
			_, err := getWaitStatus(context.Background(), client, server.URL, "", "job")
			if err == nil || clioutput.Describe(err).Code != tc.want || strings.Contains(clioutput.Describe(err).Message, "secret") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestWaitInvalidArgumentsAndPausedJob(t *testing.T) {
	for _, args := range [][]string{{"wait", "--json"}, {"wait", "--json", "--timeout=-1s", "job"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := newOutputTestRoot(t)
			root.AddCommand(newWaitCommand())
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, args, &out, &diagnostic); code != 2 || !json.Valid(out.Bytes()) || diagnostic.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &diagnostic)
			}
		})
	}
	items, err := observeDownloads(context.Background(), []string{"paused"}, func(context.Context, string) (*types.DownloadStatus, error) {
		return &types.DownloadStatus{ID: "paused", Status: "paused"}, nil
	}, time.Second, "local", "")
	if clioutput.ExitCode(err) != 7 || items[0].Artifact != nil {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	command := newWaitCommand()
	command.SetOut(&brokenOutput{})
	if err := writeWaitResult(command, items, errors.New("failed")); err == nil {
		t.Fatal("output failure ignored")
	}
}

// ResultService is deliberately optional; the generic status API cannot safely
// stand in for it on an older service implementation.
func TestResultEndpointRequiresOwnerCapability(t *testing.T) {
	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", &httpAPITestService{})
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/download/result?id=job", nil))
	if recorder.Code != http.StatusNotImplemented || recorder.Header().Get("X-Surge-Result-Version") != "" {
		t.Fatalf("response=%+v", recorder)
	}
}

func TestWaitHumanReceiptMatchesVerifiedArtifact(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprint(verified), func(t *testing.T) {
			policy := types.ArtifactPolicy{}
			expected := "checksum not requested"
			if verified {
				policy.Verification = "verified"
				policy.SHA256 = strings.Repeat("a", 64)
				expected = "sha256 verified"
			}
			items, err := observeDownloads(context.Background(), []string{"job"}, func(context.Context, string) (*types.DownloadStatus, error) {
				return &types.DownloadStatus{ID: "job", Status: "completed", DestPath: "/model.bin", Downloaded: 42, Artifact: policy}, nil
			}, time.Second, "local", "")
			if err != nil {
				t.Fatal(err)
			}
			command := newWaitCommand()
			var out bytes.Buffer
			command.SetOut(&out)
			if err := writeWaitResult(command, items, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), expected) || (verified && strings.Contains(out.String(), "checksum not requested")) {
				t.Fatalf("misleading verification receipt: %s", out.String())
			}
		})
	}
}
