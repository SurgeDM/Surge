package cmd

import (
	"net/http"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/core"
)

const (
	defaultRemoteAPIRequestTimeout = 15 * time.Second
	defaultRemoteConnectTimeout    = 5 * time.Second
)

var (
	globalInsecureHTTP bool
	globalInsecureTLS  bool
	globalTLSCAFile    string
)

type remoteClientConfig struct {
	AllowInsecureHTTP bool
	ConnectTimeout    time.Duration
	HTTPOptions       core.HTTPClientOptions
}

func currentRemoteClientConfig() remoteClientConfig {
	return remoteClientConfig{
		AllowInsecureHTTP: globalInsecureHTTP,
		ConnectTimeout:    defaultRemoteConnectTimeout,
		HTTPOptions: core.HTTPClientOptions{
			Timeout:            defaultRemoteAPIRequestTimeout,
			InsecureSkipVerify: globalInsecureTLS,
			CAFile:             strings.TrimSpace(globalTLSCAFile),
		},
	}
}

func newRemoteDownloadService(baseURL, token string) (*core.RemoteDownloadService, error) {
	cfg := currentRemoteClientConfig()
	return core.NewRemoteDownloadService(baseURL, token, cfg.HTTPOptions)
}

func newRemoteAPIHTTPClient() (*http.Client, error) {
	cfg := currentRemoteClientConfig()
	return core.NewHTTPClient(cfg.HTTPOptions)
}
