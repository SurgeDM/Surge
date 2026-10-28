package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWatchOnlyWritesChangedSnapshots(t *testing.T) {
	for _, redraw := range []bool{false, true} {
		name := "pipe"
		if redraw {
			name = "terminal"
		}
		t.Run(name, func(t *testing.T) {
			ticks := make(chan time.Time, 3)
			for range 3 {
				ticks <- time.Time{}
			}
			close(ticks)
			frames := []string{"first\n", "first\n", "second\n", "second\n"}
			index := 0
			var out bytes.Buffer
			err := watchDownloads(context.Background(), &out, ticks, redraw, func(w io.Writer) error { _, err := io.WriteString(w, frames[index]); index++; return err })
			if err != nil || index != 4 {
				t.Fatalf("renders=%d err=%v", index, err)
			}
			expected := "first\n\nsecond\n"
			if redraw {
				expected = "\033[H\033[2Jfirst\n\033[H\033[2Jsecond\n"
			}
			if out.String() != expected {
				t.Fatalf("output=%q want=%q", out.String(), expected)
			}
		})
	}
}

func TestWatchFailurePreservesPreviousFrame(t *testing.T) {
	ticks := make(chan time.Time, 1)
	ticks <- time.Time{}
	close(ticks)
	sentinel := errors.New("refresh failed")
	renders := 0
	var out bytes.Buffer
	err := watchDownloads(context.Background(), &out, ticks, true, func(w io.Writer) error {
		renders++
		if renders == 2 {
			return sentinel
		}
		_, err := io.WriteString(w, "previous frame\n")
		return err
	})
	if !errors.Is(err, sentinel) || strings.Count(out.String(), "\033[2J") != 1 || !strings.Contains(out.String(), "previous frame") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestWatchCancellationAndWriteFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	renders := 0
	err := watchDownloads(ctx, io.Discard, make(chan time.Time), false, func(w io.Writer) error { renders++; cancel(); _, err := io.WriteString(w, "frame\n"); return err })
	if !errors.Is(err, context.Canceled) || renders != 1 {
		t.Fatalf("renders=%d err=%v", renders, err)
	}
	out := &brokenOutput{}
	err = watchDownloads(context.Background(), out, make(chan time.Time), false, func(w io.Writer) error { _, err := io.WriteString(w, "frame\n"); return err })
	if !errors.Is(err, io.ErrClosedPipe) || out.calls != 1 {
		t.Fatalf("writes=%d err=%v", out.calls, err)
	}
}

func TestWatchRedrawRequiresTerminalCapabilities(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	if supportsWatchRedraw(&bytes.Buffer{}) {
		t.Fatal("buffer is not a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if supportsWatchRedraw(w) {
		t.Fatal("pipe must not get terminal control sequences")
	}
	for _, terminal := range []string{"", "dumb", "DUMB"} {
		t.Setenv("TERM", terminal)
		if supportsWatchRedraw(os.Stdout) {
			t.Fatal("terminal without capabilities must use plain output")
		}
	}
}

func TestListingExternalMetadataCannotControlTerminal(t *testing.T) {
	input := "\033[2Jmodel\t\n.gguf\033]8;;https://example.com\a link\033]8;;\a"
	if got := safeTerminalText(input); got != "model.gguf link" {
		t.Fatalf("unsafe or altered text: %q", got)
	}
}
