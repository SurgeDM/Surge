//go:build windows

package state

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
// file-locking errors (e.g. antivirus scanners, delayed handle release).
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
