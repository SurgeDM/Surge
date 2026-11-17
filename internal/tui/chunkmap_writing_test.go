package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestWritingRangesFollowNewWrites(t *testing.T) {
	d := NewDownloadModel("writing", "", "", 1000)
	now := time.Now()
	d.updateWritingRanges([]types.Task{{Offset: 100, Length: 100}, {Offset: 700, Length: 100}}, now)
	if len(d.writingRanges) != 0 {
		t.Fatal("restored progress must not count as a new write")
	}
	// One worker advances and another joins the two completed ranges.
	d.updateWritingRanges([]types.Task{{Offset: 100, Length: 750}}, now)
	want := []types.Task{{Offset: 200, Length: 500}, {Offset: 800, Length: 50}}
	if !reflect.DeepEqual(d.writingRanges, want) {
		t.Fatalf("writing = %v, want %v", d.writingRanges, want)
	}
	d.updateWritingRanges([]types.Task{{Offset: 100, Length: 750}}, now.Add(time.Second))
	if !d.lastMapWrite.Equal(now) {
		t.Fatal("unchanged progress must not extend the write highlight")
	}
}
