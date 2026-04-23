package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"

	"github.com/SurgeDM/Surge/internal/config"
	enginetypes "github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

type requestHeadersContextKey struct{}

type transportProfile struct {
	ProxyURL   string
	CustomDNS  string
	MaxConns   int
	ForceTCP4  bool
	ForceHTTP1 bool
}

type clientProfile struct {
	Transport transportProfile
	Mode      string
	UserAgent string
}

// ConnectionManager owns shared transports and clients for download/probe traffic.
type ConnectionManager struct {
	mu         sync.Mutex
	transports map[transportProfile]*http.Transport
	clients    map[clientProfile]*http.Client
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		transports: make(map[transportProfile]*http.Transport),
		clients:    make(map[clientProfile]*http.Client),
	}
}

func WithRequestHeaders(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	return context.WithValue(ctx, requestHeadersContextKey{}, headers)
}

func (m *ConnectionManager) ConcurrentClient(runtime *enginetypes.RuntimeConfig) *http.Client {
	if runtime == nil {
		runtime = &enginetypes.RuntimeConfig{}
	}

	transportProfile := transportProfile{
		ProxyURL:   strings.TrimSpace(runtime.ProxyURL),
		CustomDNS:  strings.TrimSpace(runtime.CustomDNS),
		MaxConns:   runtime.GetMaxConnectionsPerHost(),
		ForceHTTP1: true,
	}
	clientProfile := clientProfile{
		Transport: transportProfile,
		Mode:      "concurrent",
		UserAgent: runtime.GetUserAgent(),
	}
	return m.client(clientProfile)
}

func (m *ConnectionManager) ProbeClient(runtime *config.RuntimeConfig) *http.Client {
	if runtime == nil {
		runtime = &config.RuntimeConfig{}
	}

	transportProfile := transportProfile{
		ProxyURL:   strings.TrimSpace(runtime.ProxyURL),
		CustomDNS:  strings.TrimSpace(runtime.CustomDNS),
		MaxConns:   runtime.MaxConnectionsPerHost,
		ForceHTTP1: true,
		ForceTCP4:  true,
	}
	if transportProfile.MaxConns <= 0 {
		transportProfile.MaxConns = enginetypes.PerHostMax
	}

	clientProfile := clientProfile{
		Transport: transportProfile,
		Mode:      "probe",
		UserAgent: defaultUserAgent(runtime.UserAgent),
	}
	return m.client(clientProfile)
}

func (m *ConnectionManager) CloseIdleConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		client.CloseIdleConnections()
	}
}

func (m *ConnectionManager) Shutdown() {
	m.CloseIdleConnections()
}

func (m *ConnectionManager) client(profile clientProfile) *http.Client {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, ok := m.clients[profile]; ok {
		return client
	}

	transport := m.transportLocked(profile.Transport)
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if len(via) > 0 {
				utils.CopyRedirectHeaders(req, via[0])
			}
			if customHeaders, ok := req.Context().Value(requestHeadersContextKey{}).(map[string]string); ok {
				for k, v := range customHeaders {
					if !strings.EqualFold(k, "Range") {
						req.Header.Set(k, v)
					}
				}
			}
			return nil
		},
	}

	m.clients[profile] = client
	return client
}

func (m *ConnectionManager) transportLocked(profile transportProfile) *http.Transport {
	if transport, ok := m.transports[profile]; ok {
		return transport
	}

	proxyFunc := http.ProxyFromEnvironment
	if profile.ProxyURL != "" {
		if parsedURL, err := neturl.Parse(profile.ProxyURL); err == nil {
			proxyFunc = http.ProxyURL(parsedURL)
		} else {
			utils.Debug("Invalid proxy URL %s: %v", profile.ProxyURL, err)
		}
	}

	dialer := &net.Dialer{
		Timeout:   enginetypes.DialTimeout,
		KeepAlive: enginetypes.KeepAliveDuration,
	}
	utils.ConfigureDialer(dialer, profile.CustomDNS)

	maxConns := profile.MaxConns
	if maxConns <= 0 {
		maxConns = enginetypes.PerHostMax
	}

	transport := &http.Transport{
		MaxIdleConns:          enginetypes.DefaultMaxIdleConns,
		MaxIdleConnsPerHost:   maxConns + enginetypes.DialHedgeCount + 2,
		MaxConnsPerHost:       maxConns,
		Proxy:                 proxyFunc,
		IdleConnTimeout:       enginetypes.DefaultIdleConnTimeout,
		TLSHandshakeTimeout:   enginetypes.DefaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: enginetypes.DefaultResponseHeaderTimeout,
		ExpectContinueTimeout: enginetypes.DefaultExpectContinueTimeout,
		DisableCompression:    true,
	}

	if profile.ForceHTTP1 {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	}

	if profile.ForceTCP4 {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if network == "tcp" {
				network = "tcp4"
			}
			return dialer.DialContext(ctx, network, addr)
		}
	} else {
		transport.DialContext = dialer.DialContext
	}

	m.transports[profile] = transport
	return transport
}

func defaultUserAgent(ua string) string {
	if strings.TrimSpace(ua) != "" {
		return ua
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
}
