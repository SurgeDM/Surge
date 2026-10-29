package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

func executeFetchTest(t *testing.T, ctx context.Context, args ...string) (int, fetchResult, string) {
	t.Helper()
	root := &cobra.Command{Use: "surge", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(ctx)
	root.AddCommand(newFetchCommand())
	var out, diagnostic bytes.Buffer
	code := runCommand(root, append([]string{"fetch", "--json"}, args...), &out, &diagnostic)
	var result fetchResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid result: %s / %s: %v", &out, &diagnostic, err)
	}
	if diagnostic.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", &diagnostic)
	}
	return code, result, out.String()
}

func TestFetchEmbeddedVerificationAndNoClobber(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	payload := []byte("verified artifact bytes")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	source := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	}))
	defer source.Close()
	directory := t.TempDir()
	code, result, output := executeFetchTest(t, context.Background(), "--output", directory, "--filename", "model.bin", "--sha256", digest, source.URL+"/signed?token=secret")
	if code != 0 || !result.OK || result.SessionID == "" || len(result.Items) != 1 || result.Items[0].Artifact == nil || result.Items[0].Artifact.Verification != "verified" || result.Items[0].Artifact.SHA256 != digest || strings.Contains(output, "secret") {
		t.Fatalf("exit=%d result=%+v output=%s", code, result, output)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "model.bin")); err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("artifact=%q err=%v", data, err)
	}
	code, result, _ = executeFetchTest(t, context.Background(), "--output", directory, "--filename", "model.bin", source.URL)
	if code != 7 || result.OK {
		t.Fatalf("existing destination accepted: exit=%d %+v", code, result)
	}
	if data, _ := os.ReadFile(filepath.Join(directory, "model.bin")); !bytes.Equal(data, payload) {
		t.Fatal("existing artifact overwritten")
	}
}

func TestFetchIntegrityMismatchPreservesWorkingFile(t *testing.T) {
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
	code, result, _ := executeFetchTest(t, context.Background(), "--output", directory, "--filename", "mismatch.bin", "--sha256", strings.Repeat("0", 64), source.URL)
	if code != 7 || result.OK || result.Items[0].Artifact != nil || result.Items[0].Error == nil {
		t.Fatalf("mismatch accepted: exit=%d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(directory, "mismatch.bin")); !os.IsNotExist(err) {
		t.Fatal("unverified final file was published")
	}
	if data, err := os.ReadFile(filepath.Join(directory, "mismatch.bin.surge")); err != nil || string(data) != "abc" {
		t.Fatalf("working data was not retained: %q %v", data, err)
	}
}

func TestFetchInterruptCheckpointsAndResumesOwnedSession(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	payload := bytes.Repeat([]byte("resume-me"), 8192)
	digest := sha256.Sum256(payload)
	began, release := make(chan struct{}), make(chan struct{})
	var beganOnce sync.Once
	source := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, end := 0, len(payload)-1
		if value := r.Header.Get("Range"); value != "" {
			parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
			start, _ = strconv.Atoi(parts[0])
			if parts[1] != "" {
				end, _ = strconv.Atoi(parts[1])
			}
			end = min(end, len(payload)-1)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
			w.WriteHeader(206)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		}
		if r.Method == http.MethodHead {
			return
		}
		if end-start+1 == 1 {
			_, _ = w.Write(payload[start : end+1])
			return
		}
		mid := min(start+4096, end+1)
		_, _ = w.Write(payload[start:mid])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		beganOnce.Do(func() { close(began) })
		select {
		case <-release:
			_, _ = w.Write(payload[mid : end+1])
		case <-r.Context().Done():
		}
	}))
	defer source.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-began; cancel() }()
	directory := t.TempDir()
	code, first, _ := executeFetchTest(t, ctx, "--output", directory, "--filename", "resume.bin", "--sha256", hex.EncodeToString(digest[:]), source.URL)
	if code != 130 || first.CheckpointConfirmed == nil || !*first.CheckpointConfirmed || first.SessionID == "" {
		t.Fatalf("interruption did not checkpoint: exit=%d %+v", code, first)
	}
	close(release)
	code, resumed, _ := executeFetchTest(t, context.Background(), "--resume", first.SessionID)
	if code != 0 || !resumed.OK || resumed.Items[0].Artifact == nil || resumed.Items[0].Artifact.Verification != "verified" {
		t.Fatalf("resume failed: exit=%d %+v", code, resumed)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "resume.bin")); err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("resumed bytes differ: %d %v", len(data), err)
	}
}

func TestRemoteFetchTimeoutDoesNotMutateDaemonJobs(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	var mutations atomic.Int32
	const id = "12345678-1234-1234-1234-123456789012"
	queried := make(chan struct{})
	var once sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/capabilities":
			_ = json.NewEncoder(w).Encode(serverCapabilities{SchemaVersion: 1, Features: map[string]bool{"finalized_results": true, "destination_policy": true}})
		case "/download":
			mutations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case "/download/result":
			once.Do(func() { close(queried); cancel() })
			<-r.Context().Done()
		default:
			mutations.Add(1)
			http.Error(w, "unexpected mutation", 500)
		}
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	code, result, _ := executeFetchTest(t, ctx, "https://example.com/model")
	if code != 130 || result.SessionID != "" || mutations.Load() != 1 || len(result.Items) != 1 || result.Items[0].ID != id {
		t.Fatalf("observer changed daemon ownership: exit=%d mutations=%d %+v", code, mutations.Load(), result)
	}
}

func TestRemoteFetchReplayDoesNotInventInvocationTransferSpeed(t *testing.T) {
	for _, replayed := range []bool{false, true} {
		name := "fresh"
		if replayed {
			name = "replayed"
		}
		t.Run(name, func(t *testing.T) {
			setupIsolatedCmdState(t)
			resetCommandConnectionState(t)
			const id = "12345678-1234-1234-1234-123456789012"
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/capabilities":
					_ = json.NewEncoder(w).Encode(serverCapabilities{SchemaVersion: 1, Features: map[string]bool{"finalized_results": true, "destination_policy": true, "idempotency": true}})
				case "/download":
					if r.Header.Get("Idempotency-Key") == "" {
						t.Error("missing durable key")
					}
					if replayed {
						w.Header().Set("Idempotency-Replayed", "true")
					}
					_ = json.NewEncoder(w).Encode(submissionReceipt{ID: id})
				case "/download/result":
					w.Header().Set("X-Surge-Result-Version", "1")
					_ = json.NewEncoder(w).Encode(types.DownloadStatus{ID: id, Status: "completed", DestPath: "/remote/model.bin", Downloaded: 1024, TotalSize: 1024})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			globalHost, globalToken = server.URL, "test-token"
			code, result, _ := executeFetchTest(t, context.Background(), "--idempotency-key", "model", "https://example.com/model")
			if code != 0 || len(result.Items) != 1 || result.Items[0].Artifact == nil {
				t.Fatalf("remote result: exit=%d %+v", code, result)
			}
			item := result.Items[0]
			if item.ReusedBytesExact == replayed || (item.AverageBytesPerSecond == nil) != replayed {
				t.Fatalf("replayed=%t has misleading invocation metrics: %+v", replayed, item)
			}
		})
	}
}
