package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/spf13/cobra"
)

func TestValidateLimitArgs(t *testing.T) {
	tests := []struct {
		name       string
		global     bool
		defaultSet bool
		args       []string
		wantErr    string
	}{
		{name: "download limit", args: []string{"download-id", "1MB/s"}},
		{name: "global limit", global: true, args: []string{"1MB/s"}},
		{name: "default limit", defaultSet: true, args: []string{"1MB/s"}},
		{name: "both scopes", global: true, defaultSet: true, args: []string{"1MB/s"}, wantErr: "use only one"},
		{name: "missing scoped rate", global: true, wantErr: "provide exactly one"},
		{name: "missing download rate", args: []string{"download-id"}, wantErr: "provide a download ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("global", tt.global, "")
			cmd.Flags().Bool("default", tt.defaultSet, "")

			err := validateLimitArgs(cmd, tt.args)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validateLimitArgs() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("validateLimitArgs() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunLimitCommandValidatesRateBeforeConnecting(t *testing.T) {
	setupIsolatedCmdState(t)
	previousHost := globalHost
	globalHost = "http://127.0.0.1:1"
	t.Cleanup(func() { globalHost = previousHost })

	cmd := &cobra.Command{}
	cmd.Flags().Bool("global", true, "")
	err := runLimitCommand(cmd, []string{"not-a-rate"})
	if err == nil || !strings.Contains(err.Error(), "rate limit missing numeric value") {
		t.Fatalf("runLimitCommand() error = %v, want invalid-rate error", err)
	}
}

func TestRunLimitCommandSendsEscapedDownloadRequest(t *testing.T) {
	setupIsolatedCmdState(t)
	const id = "aabbccdd-1234-5678-90ab-cdef12345678"
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rate-limit" {
			t.Fatalf("request = %s %s, want POST /rate-limit", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != id {
			t.Errorf("id = %q, want %q", got, id)
		}
		if got := r.URL.Query().Get("rate"); got != "2000000" {
			t.Errorf("rate = %q, want 2000000", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousHost, previousToken := globalHost, globalToken
	globalHost, globalToken = server.URL, "test-token"
	t.Cleanup(func() { globalHost, globalToken = previousHost, previousToken })

	output := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(output)
	err := runLimitCommand(cmd, []string{id, "2MB/s"})
	if err != nil {
		t.Fatalf("runLimitCommand() error = %v", err)
	}
	if got := output.String(); got != "Set speed limit for "+id+" to 1.9 MiB/s\n" {
		t.Errorf("output = %q", got)
	}
}

func TestExecuteLimitRequestBoundsErrorBody(t *testing.T) {
	body := strings.Repeat("x", maxLimitErrorResponseBytes+100)
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, body, http.StatusInternalServerError)
	}))
	defer server.Close()

	err := executeLimitRequest(server.URL, "", "/rate-limit")
	if err == nil {
		t.Fatal("executeLimitRequest() returned nil error")
	}
	message := err.Error()
	if !strings.Contains(message, strings.Repeat("x", maxLimitErrorResponseBytes)) || !strings.HasSuffix(message, "...") {
		t.Errorf("error does not contain a truncated response body: %q", message)
	}
	if strings.Contains(message, strings.Repeat("x", maxLimitErrorResponseBytes+1)) {
		t.Errorf("error contains more than %d response bytes", maxLimitErrorResponseBytes)
	}
}

func TestRateLimitPathEscapesQueryValues(t *testing.T) {
	got := rateLimitPath("/rate-limit", map[string][]string{"id": {"id&value"}, "inherit": {"true"}})
	want := "/rate-limit?id=id%26value&inherit=true"
	if got != want {
		t.Errorf("rateLimitPath() = %q, want %q", got, want)
	}
}
