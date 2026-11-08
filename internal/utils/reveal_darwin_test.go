package utils

import (
	"slices"
	"testing"
)

func TestBuildRevealCommand(t *testing.T) {
	cmd := buildRevealCommand("/tmp/My Files/report 1.pdf")
	want := []string{"open", "-R", "/tmp/My Files/report 1.pdf"}
	if cmd == nil || !slices.Equal(cmd.Args, want) {
		t.Fatalf("expected %v, got %v", want, cmd)
	}
}
