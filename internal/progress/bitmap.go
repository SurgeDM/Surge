package progress

import (
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
	"sync"
)

type BitmapTracker struct {
	mu              sync.Mutex
	bitmap          []byte
	chunkProgress   []int64
	actualChunkSize int64
	width           int
}

// bitmapLayout returns the number of tracked chunks and backing bytes for a
// 2-bit-per-chunk bitmap.
func bitmapLayout(totalSize, chunkSize int64) (numChunks int, bytesNeeded int, ok bool) {
	if totalSize <= 0 || chunkSize <= 0 {
		return 0, 0, false
	}

	numChunks = int((totalSize + chunkSize - 1) / chunkSize)
	if numChunks <= 0 {
		return 0, 0, false
	}

	bytesNeeded = (numChunks + 3) / 4
	return numChunks, bytesNeeded, true
}

// Reset clears the bitmap completely.
func (b *BitmapTracker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.width > 0 {
		b.bitmap = make([]byte, len(b.bitmap))
		b.chunkProgress = make([]int64, b.width)
	}
}

// InitBitmap initializes the chunk bitmap.
func (b *BitmapTracker) InitBitmap(totalSize int64, chunkSize int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.bitmap) > 0 && b.actualChunkSize == chunkSize {
		// Already initialized and the chunk size is correct.
		// NOTE: TotalSize check is left to the caller or implicitly valid.
		return
	}

	utils.Debug("InitBitmap: Total=%d, ChunkSize=%d", totalSize, chunkSize)
	if chunkSize <= 0 {
		return
	}

	numChunks, bytesNeeded, ok := bitmapLayout(totalSize, chunkSize)
	if !ok {
		return
	}

	b.actualChunkSize = chunkSize
	b.width = numChunks
	b.bitmap = make([]byte, bytesNeeded)
	b.chunkProgress = make([]int64, numChunks)
}

// RestoreBitmap restores the chunk bitmap from saved state.
func (b *BitmapTracker) RestoreBitmap(totalSize int64, bitmap []byte, actualChunkSize int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(bitmap) == 0 || actualChunkSize <= 0 || totalSize <= 0 {
		return
	}

	numChunks, bytesNeeded, ok := bitmapLayout(totalSize, actualChunkSize)
	if !ok {
		return
	}

	b.bitmap = make([]byte, bytesNeeded)
	copy(b.bitmap, bitmap)
	b.actualChunkSize = actualChunkSize
	b.width = numChunks

	if len(b.chunkProgress) != numChunks {
		b.chunkProgress = make([]int64, numChunks)
	}
}

// SetChunkProgress updates chunk progress array from external sources.
func (b *BitmapTracker) SetChunkProgress(progress []int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(progress) == 0 {
		return
	}
	if len(b.chunkProgress) != len(progress) {
		b.chunkProgress = make([]int64, len(progress))
	}
	copy(b.chunkProgress, progress)
}

// SetChunkState sets the 2-bit state for a specific chunk index.
func (b *BitmapTracker) SetChunkState(index int, status types.ChunkStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setChunkState(index, status)
}

// setChunkState sets the 2-bit state (internal, expects lock).
func (b *BitmapTracker) setChunkState(index int, status types.ChunkStatus) {
	if index < 0 || index >= b.width {
		return
	}

	byteIndex := index / 4
	if byteIndex >= len(b.bitmap) {
		return
	}
	bitOffset := (index % 4) * 2

	mask := byte(3 << bitOffset)
	b.bitmap[byteIndex] &= ^mask

	val := byte(status) << bitOffset
	b.bitmap[byteIndex] |= val
}

// GetChunkState gets the 2-bit state for a specific chunk index.
func (b *BitmapTracker) GetChunkState(index int) types.ChunkStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.getChunkState(index)
}

func (b *BitmapTracker) getChunkState(index int) types.ChunkStatus {
	if index < 0 || index >= b.width {
		return types.ChunkPending
	}

	byteIndex := index / 4
	if byteIndex >= len(b.bitmap) {
		return types.ChunkPending
	}
	bitOffset := (index % 4) * 2

	val := (b.bitmap[byteIndex] >> bitOffset) & 3
	return types.ChunkStatus(val)
}

// UpdateChunkStatus updates the bitmap based on byte range and returns the incremented progress.
func (b *BitmapTracker) UpdateChunkStatus(totalSize, offset, length int64, status types.ChunkStatus) (increment int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.actualChunkSize == 0 || len(b.bitmap) == 0 {
		return 0
	}

	if len(b.chunkProgress) != b.width {
		utils.Debug("UpdateChunkStatus: Initializing ChunkProgress array (width=%d)", b.width)
		b.chunkProgress = make([]int64, b.width)
	}

	startIdx := int(offset / b.actualChunkSize)
	endIdx := int((offset + length - 1) / b.actualChunkSize)

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= b.width {
		endIdx = b.width - 1
	}

	var totalIncrement int64

	for i := startIdx; i <= endIdx; i++ {
		chunkStart := int64(i) * b.actualChunkSize
		chunkEnd := chunkStart + b.actualChunkSize
		if chunkEnd > totalSize {
			chunkEnd = totalSize
		}

		updateStart := offset
		if updateStart < chunkStart {
			updateStart = chunkStart
		}

		updateEnd := offset + length
		if updateEnd > chunkEnd {
			updateEnd = chunkEnd
		}

		overlap := updateEnd - updateStart
		if overlap < 0 {
			overlap = 0
		}

		switch status {
		case types.ChunkCompleted:
			inc := overlap
			remainingSpace := (chunkEnd - chunkStart) - b.chunkProgress[i]

			if inc > remainingSpace {
				inc = remainingSpace
			}

			if inc > 0 {
				b.chunkProgress[i] += inc
				totalIncrement += inc
			}

			if b.chunkProgress[i] >= (chunkEnd - chunkStart) {
				b.chunkProgress[i] = chunkEnd - chunkStart
				b.setChunkState(i, types.ChunkCompleted)
			} else {
				if b.getChunkState(i) != types.ChunkCompleted {
					b.setChunkState(i, types.ChunkDownloading)
				}
			}
		case types.ChunkDownloading:
			current := b.getChunkState(i)
			if current != types.ChunkCompleted {
				b.setChunkState(i, types.ChunkDownloading)
			}
		}
	}

	return totalIncrement
}

// RecalculateProgress reconstructs ChunkProgress from remaining tasks and returns total verified bytes.
func (b *BitmapTracker) RecalculateProgress(totalSize int64, remainingTasks []types.Task) (totalVerified int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.actualChunkSize == 0 || b.width == 0 {
		return 0
	}

	b.chunkProgress = make([]int64, b.width)
	var total int64
	for i := 0; i < b.width; i++ {
		chunkStart := int64(i) * b.actualChunkSize
		chunkEnd := chunkStart + b.actualChunkSize
		if chunkEnd > totalSize {
			chunkEnd = totalSize
		}
		b.chunkProgress[i] = chunkEnd - chunkStart
		total += b.chunkProgress[i]
	}

	for _, task := range remainingTasks {
		offset := task.Offset
		length := task.Length

		startIdx := int(offset / b.actualChunkSize)
		endIdx := int((offset + length - 1) / b.actualChunkSize)

		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx >= b.width {
			endIdx = b.width - 1
		}

		for i := startIdx; i <= endIdx; i++ {
			chunkStart := int64(i) * b.actualChunkSize
			chunkEnd := chunkStart + b.actualChunkSize
			if chunkEnd > totalSize {
				chunkEnd = totalSize
			}

			taskStart := offset
			if taskStart < chunkStart {
				taskStart = chunkStart
			}

			taskEnd := offset + length
			if taskEnd > chunkEnd {
				taskEnd = chunkEnd
			}

			overlap := taskEnd - taskStart
			if overlap > 0 {
				b.chunkProgress[i] -= overlap
				total -= overlap
			}
		}
	}

	for i := 0; i < b.width; i++ {
		chunkStart := int64(i) * b.actualChunkSize
		chunkEnd := chunkStart + b.actualChunkSize
		if chunkEnd > totalSize {
			chunkEnd = totalSize
		}
		chunkSize := chunkEnd - chunkStart

		if b.chunkProgress[i] >= chunkSize {
			b.chunkProgress[i] = chunkSize
			b.setChunkState(i, types.ChunkCompleted)
		} else if b.chunkProgress[i] > 0 {
			b.setChunkState(i, types.ChunkDownloading)
		} else {
			b.chunkProgress[i] = 0
			b.setChunkState(i, types.ChunkPending)
		}
	}
	return total
}

// GetBitmapSnapshot returns a copy of bitmap metadata and optionally chunk progress.
func (b *BitmapTracker) GetBitmapSnapshot(totalSize int64, includeProgress bool) ([]byte, int, int64, int64, []int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.bitmap) == 0 {
		return nil, 0, 0, 0, nil
	}

	result := make([]byte, len(b.bitmap))
	copy(result, b.bitmap)

	var progressResult []int64
	if includeProgress {
		progressResult = make([]int64, len(b.chunkProgress))
		copy(progressResult, b.chunkProgress)
	}

	return result, b.width, totalSize, b.actualChunkSize, progressResult
}
