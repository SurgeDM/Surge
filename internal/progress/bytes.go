package progress

import "sync/atomic"

// ByteTracker handles thread-safe lock-free byte counting.
type ByteTracker struct {
	Downloaded       atomic.Int64
	VerifiedProgress atomic.Int64
	TotalSize        int64 // Immutable after initialization via SetTotalSize
}

// SetTotalSize initializes the total size.
func (b *ByteTracker) SetTotalSize(size int64) {
	b.TotalSize = size
}
