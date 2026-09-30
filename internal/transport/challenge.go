package transport

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
)

type replayBody struct {
	io.Reader
	io.Closer
}

// IsServerChallenge recognizes explicit mitigation headers or challenge body
// signatures. CDN identity and compression alone do not establish a challenge.
// Any inspected bytes are replayed so an ordinary response stays readable.
func IsServerChallenge(resp *http.Response) (bool, error) {
	if strings.EqualFold(resp.Header.Get("Cf-Mitigated"), "challenge") {
		return true, nil
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return false, nil
	}
	prefix, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	original := resp.Body
	resp.Body = &replayBody{Reader: io.MultiReader(bytes.NewReader(prefix), original), Closer: original}
	if err != nil {
		return false, err
	}
	var decoder io.ReadCloser
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		decoder, err = gzip.NewReader(bytes.NewReader(prefix))
	case "deflate":
		decoder, err = zlib.NewReader(bytes.NewReader(prefix))
	}
	if err != nil {
		return false, nil
	}
	if decoder != nil {
		decoded, _ := io.ReadAll(io.LimitReader(decoder, 16*1024))
		_ = decoder.Close()
		prefix = decoded
	}
	body := strings.ToLower(string(prefix))
	return strings.Contains(body, "/cdn-cgi/challenge-platform/") ||
		strings.Contains(body, "cf-chl-") || strings.Contains(body, "_cf_chl_opt"), nil
}
