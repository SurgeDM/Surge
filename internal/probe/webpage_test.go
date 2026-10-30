package probe

import "testing"

func TestLooksLikeWebpage(t *testing.T) {
	for _, tc := range []struct {
		filename, contentType string
		want                    bool
	}{
		{"page.html", "application/octet-stream", true},
		{"notes.txt", "text/html; charset=utf-8", true},
		{"archive.zip", "application/zip", false},
		{"readme.htm", "", true},
		{"file.xhtml", "application/xhtml+xml", true},
	} {
		if got := LooksLikeWebpage(tc.filename, tc.contentType); got != tc.want {
			t.Errorf("%+v: got %v want %v", tc, got, tc.want)
		}
	}
}
