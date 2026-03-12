package tui

import (
	"time"

	"github.com/surge-downloader/surge/internal/engine/events"
)

func (m *RootModel) processProgressMsg(msg events.ProgressMsg) {
	d := m.FindDownloadByID(msg.DownloadID)
	if d == nil || d.done || d.paused {
		return
	}

	prevDownloaded := d.Downloaded
	prevElapsed := d.Elapsed

	if deltaElapsed := msg.Elapsed - prevElapsed; deltaElapsed > 0 {
		deltaDownloaded := msg.Downloaded - prevDownloaded
		if deltaDownloaded > 0 {
			d.LiveSpeed = float64(deltaDownloaded) / deltaElapsed.Seconds()
		} else {
			d.LiveSpeed = 0
		}
	} else if msg.Downloaded < prevDownloaded || msg.Elapsed < prevElapsed {
		d.LiveSpeed = 0
	}

	d.Downloaded = msg.Downloaded
	d.Total = msg.Total
	d.Speed = msg.Speed
	d.Elapsed = msg.Elapsed
	d.Connections = msg.ActiveConnections

	// Keep "Resuming..." visible until we observe actual transfer.
	if d.resuming && (d.Speed > 0 || d.Downloaded > prevDownloaded) {
		d.resuming = false
	}

	// Update Chunk State if provided
	if msg.BitmapWidth > 0 && len(msg.ChunkBitmap) > 0 {
		if d.state != nil && msg.Total > 0 {
			d.state.SetTotalSize(msg.Total)
		}
		// We only get bitmap, no progress array (to save bandwidth)
		// State needs to be updated carefully
		if d.state != nil {
			d.state.RestoreBitmap(msg.ChunkBitmap, msg.ActualChunkSize)
		}
		if d.state != nil && len(msg.ChunkProgress) > 0 {
			d.state.SetChunkProgress(msg.ChunkProgress)
		}
	}

	if d.Total > 0 {
		percentage := float64(d.Downloaded) / float64(d.Total)
		d.progress.SetPercent(percentage)
	}

	// Sample graph history from live per-interval transfer rate.
	if time.Since(m.lastSpeedHistoryUpdate) >= GraphUpdateInterval {
		totalSpeed := m.calcTotalSpeed()
		if len(m.SpeedHistory) > 0 {
			m.SpeedHistory = append(m.SpeedHistory[1:], totalSpeed)
		}
		m.lastSpeedHistoryUpdate = time.Now()
	}

	m.UpdateListItems()
}
