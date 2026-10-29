package utils

import "os"

// RenameNoReplace atomically moves a file without replacing an existing name.
// Unsupported systems return errors.ErrUnsupported; callers may use another
// atomic publication primitive, but must never fall back to os.Rename.
func RenameNoReplace(from, to string) error {
	if err := renameNoReplace(from, to); err != nil {
		return &os.LinkError{Op: "rename without replacement", Old: from, New: to, Err: err}
	}
	return nil
}
