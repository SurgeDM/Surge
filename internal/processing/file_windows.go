//go:build windows

package processing

import (
	"os"
	"time"

	"github.com/surge-downloader/surge/internal/utils"
)

const (
	retryAttempts     = 5
	retryBaseInterval = 50 * time.Millisecond
)

// retryRemove wraps os.Remove with exponential backoff for transient Windows
// file-locking errors. On Windows, a file cannot be deleted while any process
// holds an open handle; handles may linger briefly after Close() returns due
// to OS buffering, antivirus scans, or garbage collection of finalizers.
func retryRemove(path string) error {
	var err error
	wait := retryBaseInterval
	for i := 0; i < retryAttempts; i++ {
		err = os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		utils.Debug("retryRemove(%s): attempt %d failed: %v", path, i+1, err)
		time.Sleep(wait)
		wait *= 2
	}
	return err
}

// retryRename wraps os.Rename with exponential backoff for the same reason.
func retryRename(oldpath, newpath string) error {
	var err error
	wait := retryBaseInterval
	for i := 0; i < retryAttempts; i++ {
		err = os.Rename(oldpath, newpath)
		if err == nil {
			return nil
		}
		utils.Debug("retryRename(%s -> %s): attempt %d failed: %v", oldpath, newpath, i+1, err)
		time.Sleep(wait)
		wait *= 2
	}
	return err
}
