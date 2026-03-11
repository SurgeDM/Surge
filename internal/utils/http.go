package utils

import (
	"net/http"
	"strings"
)

// CopyRedirectHeaders preserves all headers for same-origin redirects
// but strips sensitive headers (cookies, auth) for cross-domain redirects.
func CopyRedirectHeaders(dst, src *http.Request) {
	if dst == nil || src == nil {
		return
	}
	sameOrigin := dst.URL != nil && src.URL != nil &&
		strings.EqualFold(dst.URL.Scheme, src.URL.Scheme) &&
		strings.EqualFold(dst.URL.Host, src.URL.Host)

	if sameOrigin {
		for key, vals := range src.Header {
			dst.Header[key] = append([]string(nil), vals...)
		}
		return
	}
	// Cross-origin: only forward safe headers
	for key := range dst.Header {
		delete(dst.Header, key)
	}
	for _, key := range []string{"Range", "User-Agent"} {
		if vals := src.Header.Values(key); len(vals) > 0 {
			dst.Header[key] = append([]string(nil), vals...)
		}
	}
}
