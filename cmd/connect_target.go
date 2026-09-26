package cmd

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type connectTarget struct {
	BaseURL string
}

func parseConnectTarget(target string, allowInsecureHTTP bool) (connectTarget, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return connectTarget{}, fmt.Errorf("invalid target: empty target")
	}

	var (
		scheme  string
		host    string
		port    string
		baseURL string
	)

	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return connectTarget{}, fmt.Errorf("invalid target %q: %w", target, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return connectTarget{}, fmt.Errorf("unsupported scheme %q (use http or https)", u.Scheme)
		}
		if u.Host == "" {
			return connectTarget{}, fmt.Errorf("invalid target %q: missing host", target)
		}
		if u.User != nil {
			return connectTarget{}, fmt.Errorf("invalid target %q: user info is not supported", target)
		}
		if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return connectTarget{}, fmt.Errorf("invalid target %q: query strings and fragments are not supported", target)
		}

		scheme = u.Scheme
		host = u.Hostname()
		port = u.Port()
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawPath = strings.TrimRight(u.RawPath, "/")
		baseURL = u.String()
	} else {
		var err error
		host, port, err = net.SplitHostPort(target)
		if err != nil {
			return connectTarget{}, formatConnectTargetAddrError(target, err)
		}
	}

	if host == "" {
		return connectTarget{}, fmt.Errorf("invalid target %q: missing host", target)
	}

	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return connectTarget{}, fmt.Errorf("invalid target %q: invalid port %q", target, port)
		}
	}

	if scheme == "" {
		scheme = "https"
		if isLoopbackHost(host) {
			scheme = "http"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, formatConnectURLHost(host, port))
	}

	if scheme == "http" && !allowInsecureHTTP && !isLoopbackHost(host) {
		return connectTarget{}, fmt.Errorf("refusing insecure HTTP for non-loopback target. Use https:// or --insecure-http")
	}

	return connectTarget{BaseURL: baseURL}, nil
}

func parseRemoteServerAddress(baseURL string) (string, int) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", 0
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = defaultPortForScheme(u.Scheme)
	}

	return u.Hostname(), port
}

func defaultPortForScheme(scheme string) int {
	switch scheme {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}

func formatConnectTargetAddrError(target string, err error) error {
	msg := err.Error()
	if strings.Contains(msg, "too many colons") {
		return fmt.Errorf("invalid target %q: IPv6 addresses with ports must use brackets, for example [2001:db8::1]:1700", target)
	}
	if strings.Contains(msg, "missing port") {
		return fmt.Errorf("invalid target %q: expected host:port or http(s) URL", target)
	}
	return fmt.Errorf("invalid target %q: %w", target, err)
}

func formatConnectURLHost(host, port string) string {
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
