//go:build !windows

package utils

import (
	"runtime"
	"slices"
	"testing"
)

func TestBuildRevealCommand(t *testing.T) {
	cmd := buildRevealCommand("/tmp/My Files/report 1.pdf")
	if runtime.GOOS != "darwin" {
		if cmd != nil {
			t.Fatalf("expected no reveal command on %s, got %v", runtime.GOOS, cmd.Args)
		}
		return
	}
	want := []string{"open", "-R", "/tmp/My Files/report 1.pdf"}
	if cmd == nil || !slices.Equal(cmd.Args, want) {
		t.Fatalf("expected %v, got %v", want, cmd)
	}
}
