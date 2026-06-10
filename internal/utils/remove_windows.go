//go:build windows

package utils

import (
	"os"
	"time"
)

const (
	removeRetryAttempts     = 5
	removeRetryBaseInterval = 50 * time.Millisecond
)

// RemoveFile removes a file from disk. On Windows it retries with exponential
// backoff to handle transient file-locking errors (antivirus scanners, delayed
// handle release from the download engine, etc.).
//
// Callers should prefer this over os.Remove for any downloaded or in-progress
// file so that Windows users do not see spurious "access denied" errors.
func RemoveFile(path string) error {
	wait := removeRetryBaseInterval
	for i := 0; ; i++ {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if i == removeRetryAttempts {
			Debug("RemoveFile(%s): final attempt %d failed: %v", path, i+1, err)
			return err
		}
		Debug("RemoveFile(%s): attempt %d failed: %v", path, i+1, err)
		time.Sleep(wait)
		wait *= 2
	}
}
