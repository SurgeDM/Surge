package cmd

import (
	"context"
	"net"
	"testing"
)

func requireTCPListener(t *testing.T) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("tcp listener unavailable: %v", err)
		return
	}
	_ = ln.Close()
}
