package cmd

import (
	"net/http"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/service"
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
	HTTPOptions       service.HTTPClientOptions
}

func currentRemoteClientConfig() remoteClientConfig {
	return remoteClientConfig{
		AllowInsecureHTTP: globalInsecureHTTP,
		ConnectTimeout:    defaultRemoteConnectTimeout,
		HTTPOptions: service.HTTPClientOptions{
			Timeout:            defaultRemoteAPIRequestTimeout,
			InsecureSkipVerify: globalInsecureTLS,
			CAFile:             strings.TrimSpace(globalTLSCAFile),
		},
	}
}

func newRemoteDownloadService(baseURL, token string) (*service.RemoteDownloadService, error) {
	cfg := currentRemoteClientConfig()
	return service.NewRemoteDownloadService(baseURL, token, cfg.HTTPOptions)
}

func newRemoteAPIHTTPClient() (*http.Client, error) {
	cfg := currentRemoteClientConfig()
	return service.NewHTTPClient(cfg.HTTPOptions)
}
