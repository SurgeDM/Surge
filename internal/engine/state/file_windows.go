//go:build windows

package state

import "github.com/SurgeDM/Surge/internal/utils"

// retryRemove delegates to utils.RemoveFile which implements exponential-backoff
// retry for transient Windows file-locking errors.
func retryRemove(path string) error {
	return utils.RemoveFile(path)
}
