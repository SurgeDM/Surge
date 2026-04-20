package backup

import (
	"path/filepath"
	"testing"
)

func TestRebaseImportedPath_RelativeTraversalIsExternalized(t *testing.T) {
	targetRoot := filepath.Join(t.TempDir(), "import-root")
	got := rebaseImportedPath(filepath.Join("..", "..", "etc", "passwd"), "", targetRoot)

	if !got.Externalized {
		t.Fatal("expected relative traversal path to be externalized")
	}
	if pathWithinRoot(targetRoot, got.Path) {
		if filepath.Clean(got.Path) == filepath.Clean(filepath.Join(targetRoot, "..", "..", "etc", "passwd")) {
			t.Fatal("expected escaped relative path to be rewritten, not preserved")
		}
	} else {
		t.Fatalf("externalized path escaped target root: %q", got.Path)
	}
}
