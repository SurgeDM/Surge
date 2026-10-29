package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameNoReplacePublishesAndPreservesExistingDestination(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "working"), filepath.Join(root, "final")
	if err := os.WriteFile(source, []byte("verified bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RenameNoReplace(source, target); errors.Is(err, errors.ErrUnsupported) {
		t.Skip("native no-replace rename unavailable")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("working name remains after rename: %v", err)
	}
	if err := os.WriteFile(source, []byte("new transfer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RenameNoReplace(source, target); !os.IsExist(err) {
		t.Fatalf("existing destination accepted: %v", err)
	}
	for name, expected := range map[string]string{source: "new transfer", target: "verified bytes"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != expected {
			t.Fatalf("publication corrupted %s: %q %v", name, data, err)
		}
	}
}
