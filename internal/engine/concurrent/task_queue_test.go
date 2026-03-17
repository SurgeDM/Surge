package concurrent

import (
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestTaskQueue_PushPop(t *testing.T) {
	q := NewTaskQueue()

	task := queuedTask{
		task:      types.Task{Offset: 0, Length: 1000},
		lineageID: 1,
	}
	q.Push(task)

	if q.Len() != 1 {
		t.Errorf("Len = %d, want 1", q.Len())
	}

	got, ok := q.Pop()
	if !ok {
		t.Error("Pop returned false, expected true")
	}
	if got.task.Offset != task.task.Offset || got.task.Length != task.task.Length || got.lineageID != task.lineageID {
		t.Errorf("Pop = %+v, want %+v", got, task)
	}
}

func TestTaskQueue_PushMultiple(t *testing.T) {
	q := NewTaskQueue()

	tasks := []queuedTask{
		{task: types.Task{Offset: 0, Length: 100}, lineageID: 1},
		{task: types.Task{Offset: 100, Length: 100}, lineageID: 2},
		{task: types.Task{Offset: 200, Length: 100}, lineageID: 3},
	}
	q.PushMultiple(tasks)

	if q.Len() != 3 {
		t.Errorf("Len = %d, want 3", q.Len())
	}
}

func TestTaskQueue_IdleWorkers(t *testing.T) {
	q := NewTaskQueue()

	// Initially 0 idle workers
	if q.IdleWorkers() != 0 {
		t.Errorf("IdleWorkers = %d, want 0", q.IdleWorkers())
	}
}

func TestTaskQueue_Close(t *testing.T) {
	q := NewTaskQueue()
	q.Push(queuedTask{task: types.Task{Offset: 0, Length: 100}, lineageID: 1})
	q.Close()

	// After close, Pop should still return existing tasks
	if _, ok := q.Pop(); !ok {
		t.Error("Pop should return existing task after Close")
	}

	// Additional Pop should return false
	if _, ok := q.Pop(); ok {
		t.Error("Pop should return false after draining closed queue")
	}
}

func TestTaskQueue_DrainRemaining(t *testing.T) {
	q := NewTaskQueue()

	tasks := []queuedTask{
		{task: types.Task{Offset: 0, Length: 100}, lineageID: 1},
		{task: types.Task{Offset: 100, Length: 100}, lineageID: 2},
		{task: types.Task{Offset: 200, Length: 100}, lineageID: 3},
	}
	q.PushMultiple(tasks)

	remaining := q.DrainRemaining()

	if len(remaining) != 3 {
		t.Errorf("DrainRemaining returned %d tasks, want 3", len(remaining))
	}
	if remaining[0].lineageID != 1 || remaining[1].lineageID != 2 || remaining[2].lineageID != 3 {
		t.Errorf("DrainRemaining lineage order = %+v", remaining)
	}
	if q.Len() != 0 {
		t.Errorf("Queue should be empty after drain, Len = %d", q.Len())
	}
}

func TestAlignedSplitSize(t *testing.T) {
	tests := []struct {
		remaining int64
		wantZero  bool
	}{
		{types.MinChunk, true},       // Too small to split (half < MinChunk)
		{2 * types.MinChunk, false},  // Half is MinChunk, valid split
		{4 * types.MinChunk, false},  // Should produce valid split
		{10 * types.MinChunk, false}, // Should produce valid split
	}

	for _, tt := range tests {
		got := alignedSplitSize(tt.remaining)
		if tt.wantZero && got != 0 {
			t.Errorf("alignedSplitSize(%d) = %d, want 0", tt.remaining, got)
		}
		if !tt.wantZero && got == 0 {
			t.Errorf("alignedSplitSize(%d) = 0, want non-zero", tt.remaining)
		}
		// Verify alignment
		if got != 0 && got%types.AlignSize != 0 {
			t.Errorf("alignedSplitSize(%d) = %d, not aligned to %d", tt.remaining, got, types.AlignSize)
		}
	}
}
