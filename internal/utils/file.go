package utils

import (
	"io"
	"os"
)

// CopyFile centralizes the rename-fallback copy path used by download finalization.
func CopyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // internal file access
	if err != nil {
		return err
	}
	defer func() {
		if cErr := in.Close(); cErr != nil {
			Debug("Error closing input file: %v", cErr)
		}
	}()

	out, err := os.Create(dst) //nolint:gosec // internal file access
	if err != nil {
		return err
	}
	defer func() {
		if cErr := out.Close(); cErr != nil {
			Debug("Error closing output file: %v", cErr)
		}
	}()

	buf := make([]byte, 1<<20)
	if _, cpErr := io.CopyBuffer(out, in, buf); cpErr != nil {
		return cpErr
	}
	return out.Sync()
}
