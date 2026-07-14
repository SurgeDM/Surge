package power

import "context"

// ReleaseFunc releases a previously acquired power-management inhibitor.
type ReleaseFunc func() error

// Controller owns OS power operations.
type Controller interface {
	Shutdown(ctx context.Context) error
	AcquireInhibitor(reason string) (ReleaseFunc, error)
}
