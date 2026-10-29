package utils

import (
	"errors"

	"golang.org/x/sys/unix"
)

func renameNoReplace(from, to string) error {
	err := unix.RenamexNp(from, to, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.ENOTSUP) {
		return errors.ErrUnsupported
	}
	return err
}
