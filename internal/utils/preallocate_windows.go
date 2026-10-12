//go:build windows

package utils

import (
	"os"

	"golang.org/x/sys/windows"
)

// preallocateFalloc marks the file sparse and then sets the logical size.
// NTFS does not zero-fill a sparse file when the end of file moves forward.
// If the sparse ioctl fails (FAT, some network shares), truncate still runs
// and the call succeeds when truncate succeeds.
// SetFileValidData is not used: it requires SeManageVolumePrivilege and can
// expose stale disk data.
func preallocateFalloc(file *os.File, size int64) error {
	// Best-effort. FAT and some network shares reject the sparse ioctl;
	// truncate still sets the logical size and the call succeeds when it does.
	if err := markSparse(file); err != nil {
		return file.Truncate(size)
	}
	return file.Truncate(size)
}

func markSparse(file *os.File) error {
	var returned uint32
	return windows.DeviceIoControl(
		windows.Handle(file.Fd()),
		windows.FSCTL_SET_SPARSE,
		nil,
		0,
		nil,
		0,
		&returned,
		nil,
	)
}

func setFallocateForTest(func(fd int, size int64) error) func() {
	return func() {}
}
