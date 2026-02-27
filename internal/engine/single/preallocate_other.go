//go:build !linux

package single

import "os"

func preallocateFile(file *os.File, size int64) error {
	if size <= 0 {
		return nil
	}
	return file.Truncate(size)
}

