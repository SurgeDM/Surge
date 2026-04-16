package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SurgeDM/Surge/internal/engine/events"
)

func main() {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		frames, err := events.EncodeSSEMessages(events.DownloadQueuedMsg{
			DownloadID: "queue-1",
			Filename:   "archive.zip",
			URL:        "https://example.com/archive.zip",
			DestPath:   "/tmp/archive.zip",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		for _, frame := range frames {
			_, _ = fmt.Fprintf(w, "event: %s\n", frame.Event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", frame.Data)
		}
	})

	server := &http.Server{Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		_ = server.Close()
	}()

	addr := listener.Addr().String()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		panic("unexpected listener address: " + addr)
	}
	fmt.Printf("READY http://%s\n", addr)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
