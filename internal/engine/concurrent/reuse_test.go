package concurrent

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/network"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func TestConcurrentDownloader_ReusesConnectionAcrossDownloads(t *testing.T) {
	var newConnections atomic.Int32

	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "reuse.bin", time.Now(), bytes.NewReader(make([]byte, 128*types.KB)))
	}))
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}

	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	exec := &types.ExecutionDeps{
		HTTPClients:    network.NewConnectionManager(),
		BufferPools:    network.NewBufferPoolManager(),
		NetworkWorkers: NewNetworkWorkerPool(1),
	}
	defer exec.NetworkWorkers.Shutdown()

	runtime := &types.RuntimeConfig{
		MaxConnectionsPerHost: 1,
		MinChunkSize:          32 * types.KB,
		WorkerBufferSize:      32 * types.KB,
	}

	for i := 0; i < 2; i++ {
		destPath := filepath.Join(tmpDir, "reuse-"+string(rune('a'+i))+".bin")
		if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
			_ = f.Close()
		}

		d := NewConcurrentDownloader("reuse-id", nil, types.NewProgressState("reuse", 128*types.KB), runtime)
		d.Execution = exec

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := d.Download(ctx, server.URL, []string{server.URL}, []string{server.URL}, destPath, 128*types.KB)
		cancel()
		if err != nil {
			t.Fatalf("download %d failed: %v", i, err)
		}
	}

	if got := newConnections.Load(); got != 1 {
		t.Fatalf("new connections = %d, want 1", got)
	}
}

func TestConcurrentDownloader_ProbeAndDownloadShareTransport(t *testing.T) {
	var newConnections atomic.Int32

	server := testutil.NewMockServerT(t,
		testutil.WithFileSize(128*types.KB),
		testutil.WithRangeSupport(true),
	)
	server.Server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}

	tmpDir, cleanup := initTestState(t)
	defer cleanup()

	manager := network.NewConnectionManager()

	exec := &types.ExecutionDeps{
		HTTPClients:    manager,
		BufferPools:    network.NewBufferPoolManager(),
		NetworkWorkers: NewNetworkWorkerPool(1),
	}
	defer exec.NetworkWorkers.Shutdown()

	probeCfg := &config.RuntimeConfig{MaxConnectionsPerHost: 1}
	if _, err := processing.ProbeServerWithProxy(context.Background(), server.URL(), "", nil, probeCfg, manager); err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	destPath := filepath.Join(tmpDir, "probe-reuse.bin")
	if f, err := os.Create(destPath + types.IncompleteSuffix); err == nil {
		_ = f.Close()
	}

	d := NewConcurrentDownloader("probe-reuse", nil, types.NewProgressState("probe-reuse", 128*types.KB), &types.RuntimeConfig{
		MaxConnectionsPerHost: 1,
		MinChunkSize:          32 * types.KB,
		WorkerBufferSize:      32 * types.KB,
	})
	d.Execution = exec

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.Download(ctx, server.URL(), []string{server.URL()}, []string{server.URL()}, destPath, 128*types.KB); err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if got := newConnections.Load(); got != 1 {
		t.Fatalf("new connections = %d, want 1", got)
	}
}
