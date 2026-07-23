package utils

import (
	"net"
	"net/http"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// SameSite checks if two hosts share the same registrable domain (eTLD+1).
func SameSite(a, b string) bool {
	aHost := a
	if h, _, err := net.SplitHostPort(a); err == nil {
		aHost = h
	}
	bHost := b
	if h, _, err := net.SplitHostPort(b); err == nil {
		bHost = h
	}

	if strings.EqualFold(aHost, bHost) {
		return true
	}

	aDomain, errA := publicsuffix.EffectiveTLDPlusOne(aHost)
	bDomain, errB := publicsuffix.EffectiveTLDPlusOne(bHost)
	if errA != nil || errB != nil {
		return false // unknown/invalid TLD — fail closed
	}
	return strings.EqualFold(aDomain, bDomain)
}

// CopyRedirectHeaders restores sensitive headers that Go's default http.Client strips
// on cross-origin redirects, provided the redirect remains on the same-site.
// For cross-site redirects, it defers to the standard library's safe defaults (which
// retain safe headers like Range/User-Agent and update Referer, but strip credentials).
func CopyRedirectHeaders(dst, src *http.Request) {
	if dst == nil || src == nil {
		return
	}
	if dst.URL != nil && src.URL != nil &&
		strings.EqualFold(dst.URL.Scheme, src.URL.Scheme) &&
		SameSite(dst.URL.Host, src.URL.Host) {
		// ponytail: We manually forward Cookie and Authorization on same-site redirects
		// instead of using a strict http.CookieJar. Why? The Surge extension only provides
		// a flattened Cookie string (e.g., "session=123"), stripping the original Domain and
		// Path attributes. If we loaded this into a CookieJar, it would default to a host-only
		// cookie for the exact starting URL, and the Jar would correctly (but fatally for us)
		// refuse to send it to same-site CDN subdomains like iad-dl-08.easynews.com.
		// We use SameSite as a safe heuristic to mimic domain-wide cookies.
		for _, k := range []string{"Cookie", "Cookie2", "Authorization"} {
			if v := src.Header.Values(k); len(v) > 0 {
				dst.Header[k] = append([]string(nil), v...)
			}
		}
	}
}
