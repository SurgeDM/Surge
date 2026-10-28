package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/charmbracelet/x/term"
)

func supportsWatchRedraw(out io.Writer) bool {
	terminal := strings.TrimSpace(os.Getenv("TERM"))
	if terminal == "" || strings.EqualFold(terminal, "dumb") {
		return false
	}
	file, ok := out.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd())
}

// Watch replaces frames only on a capable terminal. Pipes and dumb terminals
// receive append-only changed snapshots; an idle queue generates no repeated log.
// Rendering completes before any clear sequence, so a failed refresh preserves
// the previous successful view. No cursor or alternate-screen modes are changed.
func watchDownloads(ctx context.Context, out io.Writer, ticks <-chan time.Time, redraw bool, render func(io.Writer) error) error {
	previous := ""
	first := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var frame bytes.Buffer
		if err := render(&frame); err != nil {
			return err
		}
		current := frame.String()
		if first || current != previous {
			text := current
			if redraw {
				text = "\033[H\033[2J" + text
			} else if !first {
				text = "\n" + text
			}
			if err := clioutput.WriteText(out, text); err != nil {
				return err
			}
			previous = current
			first = false
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
		}
	}
}
