package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectUnsupportedStatePreservesBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.gob")
	if err := atomicWrite(path, MasterState{Version: 999}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDirectory(dir); err == nil {
		t.Fatal("unsupported version hidden")
	}
	after, err := os.Stat(path)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("inspection changed state")
	}
}
