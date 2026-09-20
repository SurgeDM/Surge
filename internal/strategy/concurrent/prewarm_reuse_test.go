package concurrent

import (
	"testing"

	"github.com/SurgeDM/Surge/internal/transport"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestSetupNetworkKeepsPerDownloadLimitOutOfSharedTransport(t *testing.T) {
	first := NewConcurrentDownloader("network-limit-first", nil, nil, &types.RuntimeConfig{
		MaxConnectionsPerDownload: 2,
	})
	second := NewConcurrentDownloader("network-limit-second", nil, nil, &types.RuntimeConfig{
		MaxConnectionsPerDownload: 7,
	})

	_, firstTransport := first.setupNetwork()
	defer transport.DefaultNetworkPool.ReleaseTransport(firstTransport)
	_, secondTransport := second.setupNetwork()
	defer transport.DefaultNetworkPool.ReleaseTransport(secondTransport)

	if firstTransport != secondTransport {
		t.Fatal("downloads with identical network settings should reuse the shared transport")
	}
	if got := firstTransport.MaxConnsPerHost; got != types.PoolMaxConnsPerHost {
		t.Fatalf("shared MaxConnsPerHost = %d, want process ceiling %d", got, types.PoolMaxConnsPerHost)
	}
}
