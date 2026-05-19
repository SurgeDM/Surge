package surge_test

import (
	"context"
	"go/build"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/pkg/surge"
)

func TestLocalServiceDownloadsFile(t *testing.T) {
	const body = "surge library download\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "none")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	surge.ConfigureStateDB(filepath.Join(t.TempDir(), "surge.db"))
	t.Cleanup(surge.CloseStateDB)

	engine, err := surge.NewLocalEngine(surge.LocalEngineOptions{MaxDownloads: 1})
	if err != nil {
		t.Fatalf("NewLocalEngine() error = %v", err)
	}
	defer func() { _ = engine.Shutdown() }()

	outDir := t.TempDir()
	id, err := engine.Service.Add(server.URL+"/file.txt", outDir, "file.txt", nil, nil, true, int64(len(body)), false)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, cleanup, err := engine.Service.StreamEvents(ctx)
	if err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}
	defer cleanup()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for download completion")
		case msg := <-stream:
			switch event := msg.(type) {
			case surge.DownloadCompleteMsg:
				if event.DownloadID != id {
					continue
				}
				finalPath := filepath.Join(outDir, "file.txt")
				waitForFile(ctx, t, finalPath)
				got, err := os.ReadFile(finalPath)
				if err != nil {
					t.Fatalf("ReadFile() error = %v", err)
				}
				if string(got) != body {
					t.Fatalf("downloaded body = %q, want %q", string(got), body)
				}
				return
			case surge.DownloadErrorMsg:
				if event.DownloadID == id {
					t.Fatalf("download failed: %v", event.Err)
				}
			}
		}
	}
}

func waitForFile(ctx context.Context, t *testing.T, path string) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", path)
		case <-ticker.C:
		}
	}
}

func TestPublicPackageHasNoUIImports(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir() error = %v", err)
	}

	for _, imp := range pkg.Imports {
		switch imp {
		case "github.com/spf13/cobra", "charm.land/bubbletea/v2":
			t.Fatalf("pkg/surge imports UI package %q", imp)
		}
	}
}
