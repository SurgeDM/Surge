package service

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HTTPClientOptions struct {
	Timeout               time.Duration
	ResponseHeaderTimeout time.Duration
	InsecureSkipVerify    bool
	CAFile                string
}

// SameOriginRedirectPolicy permits redirects only when they retain the scheme,
// hostname, and effective port of the original request.
func SameOriginRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 || sameOrigin(via[0].URL, req.URL) {
		return nil
	}
	return errors.New("refusing redirect to a different origin")
}

func sameOrigin(first, next *url.URL) bool {
	return strings.EqualFold(first.Scheme, next.Scheme) &&
		strings.EqualFold(first.Hostname(), next.Hostname()) &&
		effectivePort(first) == effectivePort(next)
}

func effectivePort(target *url.URL) string {
	if port := target.Port(); port != "" {
		return port
	}
	switch strings.ToLower(target.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	transport, err := NewHTTPTransport(opts)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
	}, nil
}

func NewStreamingHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	transport, err := NewHTTPTransport(opts)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: transport,
	}, nil
}

func NewHTTPTransport(opts HTTPClientOptions) (*http.Transport, error) {
	tlsConfig, err := newTLSConfig(opts)
	if err != nil {
		return nil, err
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}, nil
}

func newTLSConfig(opts HTTPClientOptions) (*tls.Config, error) {
	caFile := strings.TrimSpace(opts.CAFile)
	if !opts.InsecureSkipVerify && caFile == "" {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	if caFile == "" {
		return tlsConfig, nil
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system cert pool: %w", err)
	}
	if pool == nil {
		pool = x509.NewCertPool()
	}

	pemData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file %q: %w", caFile, err)
	}
	if ok := pool.AppendCertsFromPEM(pemData); !ok {
		return nil, fmt.Errorf("read CA file %q: no certificates found", caFile)
	}

	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}
