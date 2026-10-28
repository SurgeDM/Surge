package utils

import "testing"

func TestURLHostForLogRedactsCredentialsAndSignedURL(t *testing.T) {
	got := URLHostForLog("https://user:password@example.com/private/file?signature=secret#fragment")
	if got != "example.com" {
		t.Fatalf("URLHostForLog() = %q, want host only", got)
	}
}
