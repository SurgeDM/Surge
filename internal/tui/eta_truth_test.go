package tui

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestETAUpdatesBeyondTwentyFourHoursAndClearsUnknown(t *testing.T) {
	model := NewDownloadModel("id", "", "large", 3*86400)
	model.Speed = 1
	model.UpdateETA()
	if model.lastETA != 72*time.Hour {
		t.Fatalf("long ETA retained stale estimate: %v", model.lastETA)
	}
	model.Total = 0
	model.UpdateETA()
	if model.lastETA != 0 {
		t.Fatal("unknown total retained an ETA")
	}
}
func TestSnapshotHistoricalRatesNeverBecomeLiveTotalSpeed(t *testing.T) {
	model := benchModel(0)
	model.downloads = append(model.downloads, downloadModelFromStatus(types.DownloadStatus{ID: "paused", Status: "paused", AvgSpeed: 1000}, false), downloadModelFromStatus(types.DownloadStatus{ID: "active", Status: "downloading", Speed: 12, AvgSpeed: 999}, false))
	if got := model.calcTotalSpeedBps(); got != 12 {
		t.Fatalf("live speed includes history: %d", got)
	}
}
