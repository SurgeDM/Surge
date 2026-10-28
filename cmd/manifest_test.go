package cmd

import (
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
