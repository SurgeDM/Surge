package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestNetworkPool_Reuse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pool := &NetworkPool{}
	runtime := &types.RuntimeConfig{}

	// First request
	transport1 := pool.AcquireTransport(runtime.ProxyURL, runtime.CustomDNS, 0)
	client1 := &http.Client{Transport: transport1}
	req1, _ := http.NewRequest("GET", server.URL, nil)
	resp1, err := client1.Do(req1)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	_ = resp1.Body.Close()
	pool.ReleaseTransport(transport1)

	// Second request with trace
	transport2 := pool.AcquireTransport(runtime.ProxyURL, runtime.CustomDNS, 0)
	client2 := &http.Client{Transport: transport2}
	reused := false
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				reused = true
			}
		},
	}
	req2, _ := http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace), "GET", server.URL, nil)
	resp2, err := client2.Do(req2)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	_ = resp2.Body.Close()
	pool.ReleaseTransport(transport2)

	if !reused {
		t.Error("Expected connection to be reused")
	}
}

func TestNetworkPool_IdleCleanup(t *testing.T) {
	pool := &NetworkPool{}
	runtime := &types.RuntimeConfig{}

	transport := pool.AcquireTransport(runtime.ProxyURL, runtime.CustomDNS, 0)
	lease, ok := pool.transportMap[transport]
	if !ok {
		t.Fatal("Expected transport to be in transportMap")
	}

	if lease.refs != 1 {
		t.Errorf("Expected refs=1, got %d", lease.refs)
	}
	if lease.idleTimer != nil {
		t.Error("Expected no idle timer when refs > 0")
	}

	pool.ReleaseTransport(transport)
	if lease.refs != 0 {
		t.Errorf("Expected refs=0, got %d", lease.refs)
	}
	if lease.idleTimer == nil {
		t.Error("Expected idle timer to be started after ReleaseTransport()")
	}

	// Calling AcquireTransport again should stop the timer
	pool.AcquireTransport(runtime.ProxyURL, runtime.CustomDNS, 0)
	if lease.idleTimer != nil {
		t.Error("Expected idle timer to be stopped after AcquireTransport()")
	}
	pool.ReleaseTransport(transport)
}

func TestNetworkPool_ConfigChange(t *testing.T) {
	pool := &NetworkPool{}

	r1 := &types.RuntimeConfig{ProxyURL: "http://proxy1"}
	t1 := pool.AcquireTransport(r1.ProxyURL, r1.CustomDNS, 0)
	pool.ReleaseTransport(t1)

	r2 := &types.RuntimeConfig{ProxyURL: "http://proxy2"}
	t2 := pool.AcquireTransport(r2.ProxyURL, r2.CustomDNS, 0)
	pool.ReleaseTransport(t2)

	if t1 == t2 {
		t.Error("Expected different transport after config change")
	}

	// Get with same config should reuse
	t3 := pool.AcquireTransport(r2.ProxyURL, r2.CustomDNS, 0)
	pool.ReleaseTransport(t3)

	if t2 != t3 {
		t.Error("Expected transport reuse for identical config")
	}
}
