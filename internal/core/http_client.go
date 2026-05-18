package core

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type HTTPClientOptions struct {
	Timeout            time.Duration
	InsecureSkipVerify bool
	CAFile             string
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
