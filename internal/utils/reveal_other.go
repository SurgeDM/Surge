//go:build !windows && !darwin

package utils

import (
	"fmt"
	"runtime"
)

// revealFile returns an error so callers fall back to opening the folder.
func revealFile(path string) error {
	return fmt.Errorf("revealing a file is not supported on %s", runtime.GOOS)
}
