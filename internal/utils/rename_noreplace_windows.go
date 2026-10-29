package utils

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func renameNoReplace(from, to string) error {
	source, err := renamePathUTF16(from)
	if err != nil {
		return err
	}
	target, err := renamePathUTF16(to)
	if err != nil {
		return err
	}
	// Omitting MOVEFILE_REPLACE_EXISTING is essential: the destination must not
	// be replaced even when it appears after the transfer's initial checks.
	err = windows.MoveFileEx(source, target, windows.MOVEFILE_WRITE_THROUGH)
	if errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED) {
		return errors.ErrUnsupported
	}
	return err
}

// The extended absolute form keeps long paths working through the native API,
// including UNC paths. Go's os package performs equivalent normalization.
func renamePathUTF16(name string) (*uint16, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	if len(absolute) >= 248 && !strings.HasPrefix(absolute, `\\?\`) && !strings.HasPrefix(absolute, `\\.\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + strings.TrimPrefix(absolute, `\\`)
		} else {
			absolute = `\\?\` + absolute
		}
	}
	return windows.UTF16PtrFromString(absolute)
}
