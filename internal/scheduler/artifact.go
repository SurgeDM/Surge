package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Hash only when requested, after transfer writers have closed. The bounded
// buffer and context checks keep verification cancellable without any work on
// the transfer progress/rendering path.
func hashArtifact(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if readErr != nil {
			return "", readErr
		}
	}
}
