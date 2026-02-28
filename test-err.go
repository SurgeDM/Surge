//go:build ignore

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
)

func main() {
	ch := make(chan interface{}, 100)
	pool := download.NewWorkerPool(ch, 3)
	svc := core.NewLocalDownloadService(pool)
	// We must re-assign the input channel for external listeners or use the one created by NewLocalDownloadService
	// Wait, svc.InputCh is already created! Let's pass it to the pool instead.

	pool = download.NewWorkerPool(svc.InputCh, 3)
	svc.Pool = pool

	// Start listening FIRST
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, _, _ := svc.StreamEvents(ctx)

	go func() {
		for {
			select {
			case msg := <-stream:
				fmt.Printf("Event: %T\n", msg)
			case <-ctx.Done():
				return
			}
		}
	}()

	fmt.Println("Adding download...")
	id, err := svc.Add("http://localhost:12345/missing.bin", "/tmp", "missing.bin", nil, nil)
	fmt.Printf("Add returned id=%s, err=%v\n", id, err)

	time.Sleep(1 * time.Second)
	fmt.Println("Done")
}
