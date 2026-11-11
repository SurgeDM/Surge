package tui

import (
	"slices"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

// Span multiple 500ms map snapshots so the write front stays visible between updates.
const writingHighlightDuration = 1500 * time.Millisecond

func (d *DownloadModel) updateWritingRanges(current []types.Task, now time.Time) {
	if d.lastMapRanges != nil {
		advanced := newlyCompletedRanges(current, d.lastMapRanges)
		if len(advanced) > 0 {
			d.writingRanges = advanced
			d.lastMapWrite = now
		}
	}
	d.lastMapRanges = slices.Clone(current)
}

// Both inputs contain sorted, disjoint completed intervals. Subtract the old
// snapshot to find actual new writes, including ranges that join across a gap.
func newlyCompletedRanges(current, previous []types.Task) []types.Task {
	var added []types.Task
	j := 0
	for _, r := range current {
		start, end := r.Offset, r.Offset+r.Length
		for j < len(previous) && previous[j].Offset+previous[j].Length <= start {
			j++
		}
		for k := j; k < len(previous) && previous[k].Offset < end; k++ {
			old := previous[k]
			if old.Offset > start {
				added = append(added, types.Task{Offset: start, Length: old.Offset - start})
			}
			start = max(start, old.Offset+old.Length)
			if start >= end {
				break
			}
		}
		if start < end {
			added = append(added, types.Task{Offset: start, Length: end - start})
		}
	}
	return added
}
