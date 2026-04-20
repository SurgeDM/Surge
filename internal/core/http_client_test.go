package core

import (
	"strings"
	"testing"
)

func TestNewHTTPClient_InvalidCAFile(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientOptions{
		CAFile: "does-not-exist.pem",
	})
	if err == nil {
		t.Fatal("expected invalid CA file to return an error")
	}
	if !strings.Contains(err.Error(), "does-not-exist.pem") {
		t.Fatalf("error %q does not mention CA file path", err)
	}
}
