package processing

import (
	"context"
	"testing"
)

func TestEnqueue_PerTaskOverride_ZeroValues(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addFunc = func(_, _, _ string, _ []string, _ map[string]string, _ bool, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:      server.URL,
		Filename: "test.bin",
		Path:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if gotWorkers != 0 {
		t.Fatalf("expected workers=0, got %d", gotWorkers)
	}
	if gotMinChunkSize != 0 {
		t.Fatalf("expected minChunkSize=0, got %d", gotMinChunkSize)
	}
}

func TestEnqueue_PerTaskOverride_WorkersOnly(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addFunc = func(_, _, _ string, _ []string, _ map[string]string, _ bool, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     16,
		MinChunkSize: 0,
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if gotWorkers != 16 {
		t.Fatalf("expected workers=16, got %d", gotWorkers)
	}
	if gotMinChunkSize != 0 {
		t.Fatalf("expected minChunkSize=0, got %d", gotMinChunkSize)
	}
}

func TestEnqueue_PerTaskOverride_MinChunkSizeOnly(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addFunc = func(_, _, _ string, _ []string, _ map[string]string, _ bool, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     0,
		MinChunkSize: 10 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if gotWorkers != 0 {
		t.Fatalf("expected workers=0, got %d", gotWorkers)
	}
	if gotMinChunkSize != 10*1024*1024 {
		t.Fatalf("expected minChunkSize=%d, got %d", 10*1024*1024, gotMinChunkSize)
	}
}

func TestEnqueue_PerTaskOverride_BothSet(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addFunc = func(_, _, _ string, _ []string, _ map[string]string, _ bool, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     8,
		MinChunkSize: 5 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if gotWorkers != 8 {
		t.Fatalf("expected workers=8, got %d", gotWorkers)
	}
	if gotMinChunkSize != 5*1024*1024 {
		t.Fatalf("expected minChunkSize=%d, got %d", 5*1024*1024, gotMinChunkSize)
	}
}

func TestEnqueueWithID_PerTaskOverride_ZeroValues(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addWithIDFunc = func(_, _, _ string, _ []string, _ map[string]string, _ string, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:      server.URL,
		Filename: "test.bin",
		Path:     t.TempDir(),
	}, "req-1")
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if gotWorkers != 0 {
		t.Fatalf("expected workers=0, got %d", gotWorkers)
	}
	if gotMinChunkSize != 0 {
		t.Fatalf("expected minChunkSize=0, got %d", gotMinChunkSize)
	}
}

func TestEnqueueWithID_PerTaskOverride_WorkersOnly(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addWithIDFunc = func(_, _, _ string, _ []string, _ map[string]string, _ string, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     16,
		MinChunkSize: 0,
	}, "req-2")
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if gotWorkers != 16 {
		t.Fatalf("expected workers=16, got %d", gotWorkers)
	}
	if gotMinChunkSize != 0 {
		t.Fatalf("expected minChunkSize=0, got %d", gotMinChunkSize)
	}
}

func TestEnqueueWithID_PerTaskOverride_MinChunkSizeOnly(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addWithIDFunc = func(_, _, _ string, _ []string, _ map[string]string, _ string, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     0,
		MinChunkSize: 10 * 1024 * 1024,
	}, "req-3")
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if gotWorkers != 0 {
		t.Fatalf("expected workers=0, got %d", gotWorkers)
	}
	if gotMinChunkSize != 10*1024*1024 {
		t.Fatalf("expected minChunkSize=%d, got %d", 10*1024*1024, gotMinChunkSize)
	}
}

func TestEnqueueWithID_PerTaskOverride_BothSet(t *testing.T) {
	mgr := newLifecycleManagerForTest()

	var gotWorkers int
	var gotMinChunkSize int64
	mgr.addWithIDFunc = func(_, _, _ string, _ []string, _ map[string]string, _ string, workers int, minChunkSize int64, _ int64, _ bool) (string, error) {
		gotWorkers = workers
		gotMinChunkSize = minChunkSize
		return "id", nil
	}

	server := newProbeTestServer(t, 1024)
	defer server.Close()

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:         server.URL,
		Filename:    "test.bin",
		Path:        t.TempDir(),
		Workers:     8,
		MinChunkSize: 5 * 1024 * 1024,
	}, "req-4")
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if gotWorkers != 8 {
		t.Fatalf("expected workers=8, got %d", gotWorkers)
	}
	if gotMinChunkSize != 5*1024*1024 {
		t.Fatalf("expected minChunkSize=%d, got %d", 5*1024*1024, gotMinChunkSize)
	}
}
