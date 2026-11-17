package tui

import (
	"reflect"
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestProgressSnapshotPreservesExactRanges(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "remote", true: "local"}[live], func(t *testing.T) {
			d := NewDownloadModel("ranges", "https://example.com/file", "file", 1000)
			d.started = true
			d.liveProgress = live
			d.state.InitBitmap(1000, 1000)
			d.state.UpdateChunkStatus(700, 100, types.ChunkCompleted)
			m := RootModel{downloads: []*DownloadModel{d}, list: NewDownloadList(80, 20)}
			ranges := []types.Task{{Offset: 100, Length: 100}}
			m.processProgressMsg(types.DownloadEvent{
				DownloadID: d.ID, Total: 1000, Downloaded: 100,
				BitmapWidth: 1, ChunkSize: 1000, ChunkBitmap: []byte{byte(types.ChunkDownloading)},
				ChunkProgress: []int64{100}, CompletedRanges: ranges,
			})
			want := ranges
			if live {
				want = []types.Task{{Offset: 700, Length: 100}}
			}
			if got := d.state.Bitmap.GetCompletedRanges(); !reflect.DeepEqual(got, want) {
				t.Fatalf("ranges = %v, want %v", got, want)
			}
		})
	}
}
