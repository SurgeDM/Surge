//go:build !windows

package state

import "github.com/SurgeDM/Surge/internal/utils"

// retryRemove delegates to utils.RemoveFile. On non-Windows platforms this is
// a direct os.Remove call; the wrapper exists only for API consistency.
func retryRemove(path string) error {
	return utils.RemoveFile(path)
}
