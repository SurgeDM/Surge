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
		// instead of using strict browser security boundaries (like http.CookieJar).
		// Why?
		// 1. Cookies: The Surge extension provides a flattened Cookie string (e.g., "session=123")
		//    without original Domain/Path metadata. A strict CookieJar would treat this as a host-only
		//    cookie and refuse to send it to same-site CDN subdomains (breaking downloads).
		// 2. Authorization: Fetch natively drops Authorization on ALL cross-origin redirects. Real
		//    browsers only send Basic Auth to sibling origins if challenged with a matching 401 Realm.
		//    Surge lacks a 401 challenge-response engine, so we must proactively forward Auth to
		//    siblings to support authenticated CDN redirects (e.g. members.easynews.com -> iad-dl-08.easynews.com).
		// Known Ceiling: This explicitly leaks credentials to distinct sibling origins under the same
		// eTLD+1 (e.g., tenant-a.saas.com -> tenant-b.saas.com). Fixing this requires an architectural
		// rewrite to ingest full cookie metadata from the extension and build a 401 retry interceptor.
		// We use SameSite as a safe, deliberate heuristic compromise.
		for _, k := range []string{"Cookie", "Cookie2", "Authorization"} {
			if v := src.Header.Values(k); len(v) > 0 {
				dst.Header[k] = append([]string(nil), v...)
			}
		}
	}
}
