package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func TestAddReceiptsPreserveEveryInput(t *testing.T) {
	root := newOutputTestRoot(t)
	root.AddCommand(newAddCommand())
	var requests atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var request DownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" || request.Path != "/server/downloads" || !request.SkipApproval {
			t.Errorf("unexpected request: %+v", request)
		}
		switch request.URL {
		case "https://example.com/queued":
			_, _ = io.WriteString(w, `{"id":"full-download-id","status":"queued"}`)
		case "https://example.com/approval":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"approval-request-id"}`)
		default:
			http.Error(w, "https://user:secret@example.com/?token=secret", http.StatusForbidden)
		}
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	var out, diagnostic bytes.Buffer
	code := runCommand(root, []string{"get", "--output-version=1", "--output", "/server/downloads", "example.com/queued", "https://example.com/approval", "ftp://example.com/bad", "https://example.com/rejected"}, &out, &diagnostic)
	if code != 3 || diagnostic.Len() != 0 || requests.Load() != 3 {
		t.Fatalf("exit=%d requests=%d stderr=%s", code, requests.Load(), diagnostic.String())
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatalf("leaked input or server error: %s", out.String())
	}
	var result clioutput.Result[submissionItem]
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Operation != "add" || result.Error == nil || result.Error.Code != clioutput.PartialFailure || len(result.Items) != 4 {
		t.Fatalf("result: %+v", result)
	}
	for i, item := range result.Items {
		if item.InputIndex != i+1 {
			t.Fatalf("input index: %+v", item)
		}
	}
	if result.Items[0].ID != "full-download-id" || result.Items[0].RequestID != "" || result.Items[0].Status != "queued" {
		t.Fatalf("queued: %+v", result.Items[0])
	}
	if result.Items[1].RequestID != "approval-request-id" || result.Items[1].ID != "" || result.Items[1].Status != "pending_approval" {
		t.Fatalf("approval: %+v", result.Items[1])
	}
	if result.Items[2].Error.Code != clioutput.InvalidArguments || result.Items[3].Error.Code != clioutput.Unauthorized {
		t.Fatalf("failures: %+v", result.Items)
	}
}

func TestSubmissionUncertaintyDoesNotRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"empty receipt", 200, ""}, {"missing id", 200, `{}`}, {"malformed", 200, `{`},
		{"extra document", 200, `{"id":"id"} {}`}, {"oversized", 200, `{"id":"` + strings.Repeat("x", 65536) + `"}`},
		{"server failure", 503, "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newOutputTestRoot(t)
			root.AddCommand(newAddCommand())
			var requests atomic.Int32
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			globalHost, globalToken = server.URL, "test-token"
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, []string{"add", "--output-version=1", "https://example.com/file"}, &out, &diagnostic); code != 1 {
				t.Fatalf("exit=%d", code)
			}
			var result clioutput.Result[submissionItem]
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 || result.Items[0].Status != "unknown" || result.Items[0].Error == nil || result.Items[0].Error.Retryable || requests.Load() != 1 {
				t.Fatalf("result=%+v requests=%d", result, requests.Load())
			}
		})
	}
}

func TestConfirmedBatchReceipts(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		name := "approval"
		if invalid {
			name = "invalid input"
		}
		t.Run(name, func(t *testing.T) {
			root := newOutputTestRoot(t)
			root.AddCommand(newAddCommand())
			var requests atomic.Int32
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var request BatchDownloadRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if r.URL.Path != "/download/batch" || request.SkipApproval || len(request.Downloads) != 2 {
					t.Errorf("request: %+v", request)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, `{"id":"batch-approval-id"}`)
			}))
			defer server.Close()
			globalHost, globalToken = server.URL, "test-token"
			path := filepath.Join(t.TempDir(), "urls.txt")
			second := "https://example.com/b"
			if invalid {
				second = "ftp://example.com/b"
			}
			if err := os.WriteFile(path, []byte("https://example.com/a\n"+second+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic bytes.Buffer
			code := runCommand(root, []string{"add", "--output-version=1", "--batch", path, "--confirm"}, &out, &diagnostic)
			wantCode, wantRequests := 0, int32(1)
			if invalid {
				wantCode, wantRequests = 2, 0
			}
			if code != wantCode || requests.Load() != wantRequests {
				t.Fatalf("exit=%d requests=%d: %s", code, requests.Load(), diagnostic.String())
			}
			var result clioutput.Result[submissionItem]
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 2 {
				t.Fatalf("items=%+v", result.Items)
			}
			for _, item := range result.Items {
				if invalid {
					if item.Status != "not_submitted" || item.Error == nil {
						t.Fatalf("item=%+v", item)
					}
				} else if item.ID != "" || item.RequestID != "batch-approval-id" || item.Status != "pending_approval" {
					t.Fatalf("item=%+v", item)
				}
			}
		})
	}
}

func TestAddHumanReceiptsAndErrorsUseSeparateStreams(t *testing.T) {
	root := newOutputTestRoot(t)
	root.AddCommand(newAddCommand())
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"id":"full-id"}`) }))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	var out, diagnostic bytes.Buffer
	code := runCommand(root, []string{"add", "https://example.com/file", "ftp://user:secret@example.com/bad"}, &out, &diagnostic)
	if code != 1 || !strings.Contains(out.String(), "full-id") || !strings.Contains(diagnostic.String(), "Input 2:") || strings.Contains(out.String()+diagnostic.String(), "secret") {
		t.Fatalf("exit=%d out=%q err=%q", code, out.String(), diagnostic.String())
	}
}

func TestAddRequiresInputAndHonorsOutputFailures(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		root := newOutputTestRoot(t)
		root.AddCommand(newAddCommand())
		var out, diagnostic bytes.Buffer
		if code := runCommand(root, []string{"add", "--output-version=1"}, &out, &diagnostic); code != 2 {
			t.Fatalf("exit=%d", code)
		}
		var result clioutput.Result[submissionItem]
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Error == nil || result.Error.Code != clioutput.InvalidArguments {
			t.Fatalf("error=%v result=%+v", err, result)
		}
	})
	t.Run("partial receipt write", func(t *testing.T) {
		root := newOutputTestRoot(t)
		root.AddCommand(newAddCommand())
		server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"id":"id"}`) }))
		defer server.Close()
		globalHost, globalToken = server.URL, "test-token"
		out := &brokenOutput{}
		var diagnostic bytes.Buffer
		if code := runCommand(root, []string{"add", "--output-version=1", "https://example.com/file", "ftp://example.com/bad"}, out, &diagnostic); code != 1 || out.calls != 1 {
			t.Fatalf("exit=%d writes=%d", code, out.calls)
		}
	})
}
