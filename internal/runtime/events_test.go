package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestAppSubscribeAndPublish_ServiceUnavailable(t *testing.T) {
	app := NewEmpty()

	stream, cleanup, err := app.Subscribe(context.Background())
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("Subscribe() error = %v, want %v", err, ErrServiceUnavailable)
	}
	if stream != nil {
		t.Fatal("expected nil stream when service is unavailable")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when service is unavailable")
	}

	if err := app.Publish("msg"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("Publish() error = %v, want %v", err, ErrServiceUnavailable)
	}
}

func TestAppSubscribeAndPublish_UsesService(t *testing.T) {
	events := make(chan interface{}, 1)
	service := &stubDownloadService{
		streamCh: events,
		cleanup:  func() {},
	}

	app := NewEmpty()
	app.ApplyComponents(Components{Service: service})

	stream, cleanup, err := app.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if stream != events {
		t.Fatal("expected Subscribe() to return the service stream")
	}
	if cleanup == nil {
		t.Fatal("expected cleanup func")
	}

	if err := app.Publish("msg"); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if len(service.published) != 1 || service.published[0] != "msg" {
		t.Fatalf("unexpected published messages: %#v", service.published)
	}
}
