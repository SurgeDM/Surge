package transport

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIsServerChallengePreservesOrdinaryBodies(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, mitigation string
		compressed, challenge               bool
	}{
		{"ordinary CDN HTML", "text/html", "<html>download page</html>", "", false, false},
		{"compressed binary", "application/octet-stream", "normal file data", "", true, false},
		{"challenge signature", "text/html", "<script src='/cdn-cgi/challenge-platform/test'></script>", "", false, true},
		{"compressed challenge", "text/html", "<script>window._cf_chl_opt = {}</script>", "", true, true},
		{"explicit mitigation", "text/plain", "blocked", "challenge", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.body)
			if tc.compressed {
				var b bytes.Buffer
				w := gzip.NewWriter(&b)
				_, _ = w.Write(payload)
				_ = w.Close()
				payload = b.Bytes()
			}
			resp := &http.Response{Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(payload))}
			resp.Header.Set("Content-Type", tc.contentType)
			resp.Header.Set("Server", "cloudflare")
			resp.Header.Set("Cf-Mitigated", tc.mitigation)
			if tc.compressed {
				resp.Header.Set("Content-Encoding", "gzip")
			}
			challenge, err := IsServerChallenge(resp)
			if err != nil || challenge != tc.challenge {
				t.Fatalf("challenge=%v err=%v", challenge, err)
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("body changed: %q err=%v", got, err)
			}
		})
	}
}

func TestIsServerChallengeClosesOriginalBody(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader("<html>normal</html>")}
	resp := &http.Response{Header: http.Header{"Content-Type": {"text/html"}}, Body: body}
	_, _ = IsServerChallenge(resp)
	_ = resp.Body.Close()
	if !body.closed {
		t.Fatal("inspected body did not close underlying connection")
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }
