package network

import (
	"net/http"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestConnectionManager_TransportReuse(t *testing.T) {
	mgr := NewConnectionManager()
	runtime := &types.RuntimeConfig{MaxConnectionsPerHost: 8}

	c1 := mgr.ConcurrentClient(runtime)
	c2 := mgr.ConcurrentClient(runtime)

	t1, ok := c1.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected client transport to be *http.Transport")
	}
	t2, ok := c2.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected client transport to be *http.Transport")
	}

	if t1 != t2 {
		t.Fatal("expected transport reuse for identical runtime config")
	}
}

func TestConnectionManager_TransportIsolationByProxy(t *testing.T) {
	mgr := NewConnectionManager()

	c1 := mgr.ConcurrentClient(&types.RuntimeConfig{ProxyURL: "http://127.0.0.1:8080"})
	c2 := mgr.ConcurrentClient(&types.RuntimeConfig{ProxyURL: "http://127.0.0.1:9090"})

	t1 := c1.Transport.(*http.Transport)
	t2 := c2.Transport.(*http.Transport)

	if t1 == t2 {
		t.Fatal("expected different transports for different proxy settings")
	}
}

func TestConnectionManager_TransportIsolationByHedgeCount(t *testing.T) {
	mgr := NewConnectionManager()

	c1 := mgr.ConcurrentClient(&types.RuntimeConfig{DialHedgeCount: 2})
	c2 := mgr.ConcurrentClient(&types.RuntimeConfig{DialHedgeCount: 5})

	t1 := c1.Transport.(*http.Transport)
	t2 := c2.Transport.(*http.Transport)

	if t1 == t2 {
		t.Fatal("expected different transports for different hedge counts")
	}
}

func TestConnectionManager_ProbeVsConcurrentReuse(t *testing.T) {
	mgr := NewConnectionManager()
	
	downloadRuntime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 8,
		ProxyURL: "http://proxy:8080",
	}
	probeRuntime := &config.RuntimeConfig{
		MaxConnectionsPerHost: 8,
		ProxyURL: "http://proxy:8080",
	}

	cConcurrent := mgr.ConcurrentClient(downloadRuntime)
	cProbe := mgr.ProbeClient(probeRuntime)

	tConcurrent := cConcurrent.Transport.(*http.Transport)
	tProbe := cProbe.Transport.(*http.Transport)

	if tConcurrent != tProbe {
		t.Fatal("expected transport reuse between probe and concurrent clients with matching network profiles")
	}
}
