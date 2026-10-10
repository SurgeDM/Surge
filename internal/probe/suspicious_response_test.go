package probe

import "testing"

func TestSuspiciousResponseWarning(t *testing.T) {
	for _, tc := range []struct {
		filename, contentType string
		size                  int64
		warning               bool
	}{
		{"archive.7z", "text/plain; charset=utf-8", 9, true},
		{"archive.7z", "text/plain", 1469413030, false},
		{"notes.txt", "text/plain", 9, false},
		{"empty.zip", "application/zip", 22, false},
		{"document.pdf", "text/html", 200, true},
	} {
		if got := suspiciousResponseWarning(tc.filename, tc.contentType, tc.size) != ""; got != tc.warning {
			t.Errorf("%+v: warning=%v", tc, got)
		}
	}
}
