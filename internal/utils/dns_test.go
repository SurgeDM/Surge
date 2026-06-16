package utils

import (
	"net"
	"testing"
)

func TestNormalizeDNSAddr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single with port", "1.1.1.1:53", "1.1.1.1:53"},
		{"single without port defaults to 53", "1.1.1.1", "1.1.1.1:53"},
		{"comma-separated list uses first server", "1.1.1.1:53, 94.140.14.14:53", "1.1.1.1:53"},
		{"comma-separated without ports", "8.8.8.8, 8.8.4.4", "8.8.8.8:53"},
		{"leading whitespace is trimmed", "  9.9.9.9:53  ", "9.9.9.9:53"},
		{"leading comma skips the empty entry", ", 8.8.8.8:53", "8.8.8.8:53"},
		{"only separators yield empty", " , ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDNSAddr(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDNSAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// A non-empty result must be dialable. SplitHostPort catches
			// malformed values (e.g. ":53" or "[a, b]:53") that would pass the
			// string comparison above but still fail when the resolver dials.
			if got != "" {
				if _, _, err := net.SplitHostPort(got); err != nil {
					t.Errorf("normalizeDNSAddr(%q) = %q, which is not a valid host:port: %v", tt.input, got, err)
				}
			}
		})
	}
}
