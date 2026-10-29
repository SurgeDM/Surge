package utils

import (
	"io"
	"os"
)

// CopyFile centralizes the rename-fallback copy path used by download finalization.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			Debug("Error closing input file: %v", err)
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			Debug("Error closing output file: %v", err)
		}
	}()

	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(out, in, buf); err != nil {
		return err
	}
	return out.Sync()
}

// SameFilePaths reports whether two names refer to the same filesystem object.
// A missing name is distinct; other metadata errors fail closed for callers
// deciding whether opening a working path could modify a published artifact.
func SameFilePaths(first, second string) (bool, error) {
	firstInfo, err := os.Stat(first)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	secondInfo, err := os.Stat(second)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(firstInfo, secondInfo), nil
}
