package utils

import "net/url"

// URLHostForLog omits credentials, paths, queries, and fragments from a URL.
func URLHostForLog(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "<invalid>"
	}
	return parsed.Hostname()
}
