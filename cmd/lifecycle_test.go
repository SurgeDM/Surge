package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestLifecycleAllUsesOneLiveSelectionAndPreservesReceipts(t *testing.T) {
	root := newOutputTestRoot(t)
	root.AddCommand(newLifecycleCommand("pause", "Pause a download"))
	var lists atomic.Int32
	var mutations []string
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		switch r.URL.Path {
		case "/list":
			lists.Add(1)
			_ = json.NewEncoder(w).Encode([]types.DownloadStatus{
				{ID: "b", Status: "queued"}, {ID: "a", Status: "downloading"},
				{ID: "done", Status: "completed"}, {ID: "paused", Status: "paused"},
				{ID: "failed", Status: "error"}, {ID: "a", Status: "downloading"},
			})
		case "/pause":
			if r.Method != http.MethodPost {
				t.Error("expected POST")
			}
			id := r.URL.Query().Get("id")
			mutations = append(mutations, id)
			if id == "b" {
				http.Error(w, "secret source URL", http.StatusConflict)
			}
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	var out, diagnostic bytes.Buffer
	code := runCommand(root, []string{"pause", "--all", "--output-version=1"}, &out, &diagnostic)
	var result clioutput.Result[lifecycleItem]
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 3 || diagnostic.Len() != 0 || lists.Load() != 1 || strings.Join(mutations, ",") != "a,b" || strings.Contains(out.String(), "secret") {
		t.Fatalf("exit=%d lists=%d mutations=%v stdout=%s stderr=%s", code, lists.Load(), mutations, &out, &diagnostic)
	}
	if len(result.Items) != 2 || result.Items[0].Status != "requested" || result.Items[1].Error.Code != clioutput.Conflict {
		t.Fatalf("receipts: %+v", result)
	}
}

func TestLifecycleDoesNotRetryUncertainMutation(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusAccepted, http.StatusServiceUnavailable, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var count atomic.Int32
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				w.WriteHeader(status)
			}))
			defer server.Close()
			client := server.Client()
			defer client.CloseIdleConnections()
			item := requestLifecycleAction(context.Background(), client, server.URL, "", "cancel", "full-id")
			want := "requested"
			switch status {
			case http.StatusServiceUnavailable:
				want = "unknown"
			case http.StatusNotFound:
				want = "failed"
			}
			if item.Status != want || count.Load() != 1 {
				t.Fatalf("item=%+v requests=%d", item, count.Load())
			}
		})
	}
}

func TestLifecycleSnapshotFailureNeverMutates(t *testing.T) {
	root := newOutputTestRoot(t)
	root.AddCommand(newLifecycleCommand("resume", "Resume"))
	var mutations atomic.Int32
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations.Add(1)
		}
		http.Error(w, "secret", http.StatusForbidden)
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	var out, diagnostic bytes.Buffer
	if code := runCommand(root, []string{"resume", "--all", "--output-version=1"}, &out, &diagnostic); code != 5 || mutations.Load() != 0 || strings.Contains(out.String(), "secret") {
		t.Fatalf("exit=%d mutations=%d output=%s", code, mutations.Load(), &out)
	}
}

type lifecycleHTTPService struct {
	*httpAPITestService
	err             error
	paused, deleted int
}

func (s *lifecycleHTTPService) Pause(string) error  { s.paused++; return s.err }
func (s *lifecycleHTTPService) Resume(string) error { return s.err }
func (s *lifecycleHTTPService) Delete(string) error { s.deleted++; return nil }

func TestLifecycleAPIClassifiesErrorsAndCancelRetainsRecord(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{nil, 200}, {types.ErrNotFound, 404}, {types.ErrCompleted, 409},
		{types.ErrPausing, 409}, {types.ErrAlreadyActive, 409},
		{types.ErrEngineNotInit, 503}, {errors.New("failure"), 500},
	} {
		for _, path := range []string{"/pause", "/resume", "/cancel"} {
			svc := &lifecycleHTTPService{httpAPITestService: &httpAPITestService{}, err: tc.err}
			mux := http.NewServeMux()
			registerHTTPRoutes(mux, 0, "", svc)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path+"?id=job", nil))
			if recorder.Code != tc.status || svc.deleted != 0 || (path == "/cancel" && svc.paused != 1) {
				t.Fatalf("%s error=%v status=%d pause=%d delete=%d", path, tc.err, recorder.Code, svc.paused, svc.deleted)
			}
			if tc.err == nil && !strings.Contains(recorder.Body.String(), "_requested") {
				t.Fatalf("claimed checkpoint: %s", recorder.Body)
			}
		}
	}
}

func TestLifecycleCancellationAndOutputFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	item := requestLifecycleAction(ctx, nil, "", "", "pause", "id")
	if item.Status != "not_requested" || item.Error == nil {
		t.Fatalf("item=%+v", item)
	}
	command := newLifecycleCommand("pause", "Pause")
	command.SetOut(lifecycleFailingWriter{})
	if err := writeLifecycleResult(command, "pause", []lifecycleItem{{ID: "id", Status: "requested"}}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output error=%v", err)
	}
}

type lifecycleFailingWriter struct{}

func (lifecycleFailingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRemoveRejectsIDWithBulkCleanup(t *testing.T) {
	for _, flag := range []string{"clean", "clean-failed"} {
		t.Run(flag, func(t *testing.T) {
			if err := rmCmd.Flags().Set(flag, "true"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rmCmd.Flags().Set(flag, "false") })
			if err := rmCmd.RunE(rmCmd, []string{"one-job"}); err == nil || !strings.Contains(err.Error(), "cannot be used with a download ID") {
				t.Fatalf("ambiguous bulk removal was accepted: %v", err)
			}
		})
	}
}
