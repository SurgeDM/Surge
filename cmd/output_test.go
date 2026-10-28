package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

func newOutputTestRoot(t *testing.T) *cobra.Command {
	t.Helper()
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	root := &cobra.Command{Use: "surge", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newListCommand())
	return root
}

func decodeCLIResult(t *testing.T, out *bytes.Buffer) clioutput.Result[downloadItem] {
	t.Helper()
	decoder := json.NewDecoder(out)
	var result clioutput.Result[downloadItem]
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		t.Fatalf("extra output after result: %v", err)
	}
	if result.SchemaVersion != 1 || result.Operation != "ls" || result.Items == nil {
		t.Fatalf("invalid envelope: %+v", result)
	}
	return result
}

func TestListVersionedResults(t *testing.T) {
	for _, args := range [][]string{
		{"ls", "--output-version", "1"},
		{"ls", "12345678", "--output-version=1"},
		{"l", "--json", "--output-version", "1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := newOutputTestRoot(t)
			const size int64 = 9007199254740993
			entry := types.DownloadRecord{ID: "12345678-1234-1234-1234-1234567890ab", URL: "https://example.com/?token=secret", Filename: "model.bin", Status: "paused", Downloaded: size, TotalSize: size + 1}
			if err := store.AddToMasterList(entry); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, args, &out, &diagnostic); code != 0 {
				t.Fatalf("exit=%d: %s", code, diagnostic.String())
			}
			if diagnostic.Len() != 0 || strings.Contains(out.String(), "secret") {
				t.Fatalf("unexpected diagnostic or credential: %s / %s", out.String(), diagnostic.String())
			}
			result := decodeCLIResult(t, &out)
			if !result.OK || result.Error != nil || len(result.Items) != 1 {
				t.Fatalf("result: %+v", result)
			}
			item := result.Items[0]
			if item.ID != entry.ID || item.DownloadedBytes != "9007199254740993" || item.TotalBytes == nil || *item.TotalBytes != "9007199254740994" || item.SpeedBytesPerSecond != nil {
				t.Fatalf("item: %+v", item)
			}
		})
	}
}

func TestListVersionedErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		exit int
		code clioutput.Code
	}{
		{"not found", []string{"ls", "missing", "--output-version", "1"}, 4, clioutput.NotFound},
		{"extra ID", []string{"ls", "--output-version=1", "one", "two"}, 2, clioutput.InvalidArguments},
		{"watch", []string{"ls", "--output-version=1", "--watch"}, 2, clioutput.InvalidArguments},
		{"bad flag", []string{"ls", "--output-version=1", "--not-a-flag"}, 2, clioutput.InvalidArguments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newOutputTestRoot(t)
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, tc.args, &out, &diagnostic); code != tc.exit {
				t.Fatalf("exit=%d, want %d: %s", code, tc.exit, diagnostic.String())
			}
			if diagnostic.Len() != 0 {
				t.Fatalf("duplicate error diagnostic: %s", diagnostic.String())
			}
			result := decodeCLIResult(t, &out)
			if result.OK || result.Error == nil || result.Error.Code != tc.code || len(result.Items) != 0 {
				t.Fatalf("result: %+v", result)
			}
		})
	}
}

func TestListEmptyAndLegacyContracts(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		expected string
		exit     int
	}{
		{[]string{"ls"}, "No downloads found.\n", 0},
		{[]string{"ls", "--json"}, "[]\n", 0},
		{[]string{"ls", "missing", "--json"}, "", 1},
		{[]string{"ls", "--json", "--watch"}, "", 1},
		{[]string{"ls", "--output-version", "2"}, "", 2},
		{[]string{"ls", "--output-version="}, "", 2},
		// Before format negotiation, Cobra's parsing error remains plain text.
		{[]string{"ls", "--not-a-flag", "--output-version", "1"}, "", 1},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root := newOutputTestRoot(t)
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, tc.args, &out, &diagnostic); code != tc.exit {
				t.Fatalf("exit=%d, want %d", code, tc.exit)
			}
			if out.String() != tc.expected {
				t.Fatalf("stdout=%q, want %q", out.String(), tc.expected)
			}
			if (diagnostic.Len() != 0) != (tc.exit != 0) {
				t.Fatalf("stderr=%q", diagnostic.String())
			}
		})
	}
	root := newOutputTestRoot(t)
	var out, diagnostic bytes.Buffer
	if code := runCommand(root, []string{"ls", "--output-version=1"}, &out, &diagnostic); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	result := decodeCLIResult(t, &out)
	if !result.OK || len(result.Items) != 0 {
		t.Fatalf("empty result: %+v", result)
	}
}

func TestListRemoteErrorContract(t *testing.T) {
	for _, tc := range []struct {
		status    int
		code      int
		retryable bool
	}{
		{401, 5, false}, {403, 5, false}, {404, 4, false}, {409, 7, false},
		{408, 8, false}, {504, 8, false}, {429, 6, true}, {502, 6, true}, {503, 6, true}, {500, 1, false},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			root := newOutputTestRoot(t)
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "token=secret", tc.status)
			}))
			defer server.Close()
			globalHost, globalToken = server.URL, "test-token"
			var out, diagnostic bytes.Buffer
			if code := runCommand(root, []string{"ls", "--output-version=1"}, &out, &diagnostic); code != tc.code {
				t.Fatalf("exit=%d, want %d: %s", code, tc.code, diagnostic.String())
			}
			if diagnostic.Len() != 0 || strings.Contains(out.String(), "secret") {
				t.Fatalf("unsafe/duplicate diagnostic: %s / %s", out.String(), diagnostic.String())
			}
			result := decodeCLIResult(t, &out)
			if result.OK || result.Error == nil || result.Error.Retryable != tc.retryable {
				t.Fatalf("result: %+v", result)
			}
		})
	}
}

type brokenOutput struct{ calls int }

func (w *brokenOutput) Write(p []byte) (int, error) { w.calls++; return len(p) / 2, io.ErrClosedPipe }

func TestListWriteFailureIsNotReportedAsAnotherResult(t *testing.T) {
	for _, args := range [][]string{{"ls", "--output-version=1"}, {"ls", "missing", "--output-version=1"}, {"ls", "--json"}, {"ls"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := newOutputTestRoot(t)
			out := &brokenOutput{}
			var diagnostic bytes.Buffer
			if code := runCommand(root, args, out, &diagnostic); code != 1 {
				t.Fatalf("exit=%d", code)
			}
			if out.calls != 1 || !strings.Contains(diagnostic.String(), "write command output") {
				t.Fatalf("writes=%d, stderr=%q", out.calls, diagnostic.String())
			}
		})
	}
}

func TestDownloadItemsUnknownAndNonFiniteMetrics(t *testing.T) {
	for _, speed := range []float64{math.NaN(), math.Inf(1), -1} {
		items := downloadItems([]types.DownloadStatus{{Status: "downloading", Speed: speed}})
		if items[0].TotalBytes != nil || items[0].SpeedBytesPerSecond != nil {
			t.Fatalf("unknown metrics: %+v", items[0])
		}
		if err := clioutput.WriteJSON(io.Discard, clioutput.Success("ls", items)); err != nil {
			t.Fatal(err)
		}
	}
	items := downloadItems([]types.DownloadStatus{{Status: "downloading", Speed: 0}, {Status: "completed", Speed: 123}})
	if items[0].SpeedBytesPerSecond == nil || *items[0].SpeedBytesPerSecond != 0 || items[1].SpeedBytesPerSecond != nil {
		t.Fatalf("rates: %+v", items)
	}
}
