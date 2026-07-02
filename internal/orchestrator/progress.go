package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/types"
)

const (
	SpeedSmoothingAlpha = 0.3
	ReportInterval      = 150 * time.Millisecond
)

type ProgressAggregator struct {
	pool       *scheduler.Scheduler
	eventBus   *EventBus
	settingsMu sync.RWMutex
	settings   *config.Settings
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewProgressAggregator(pool *scheduler.Scheduler, eventBus *EventBus, settings *config.Settings) *ProgressAggregator {
	ctx, cancel := context.WithCancel(context.Background())
	pa := &ProgressAggregator{
		pool:     pool,
		eventBus: eventBus,
		settings: settings,
		ctx:      ctx,
		cancel:   cancel,
	}
	pa.wg.Add(1)
	go pa.reportProgressLoop()
	return pa
}

func (pa *ProgressAggregator) SetSettings(settings *config.Settings) {
	pa.settingsMu.Lock()
	pa.settings = settings
	pa.settingsMu.Unlock()
}

func (pa *ProgressAggregator) getSpeedEmaAlpha() float64 {
	pa.settingsMu.RLock()
	settings := pa.settings
	pa.settingsMu.RUnlock()

	if settings == nil {
		return SpeedSmoothingAlpha
	}

	alpha := config.Resolve[float64](settings.Performance.SpeedEmaAlpha)
	if alpha <= 0 || alpha > 1 {
		return SpeedSmoothingAlpha
	}

	return alpha
}

func (pa *ProgressAggregator) reportProgressLoop() {
	defer pa.wg.Done()
	lastSpeeds := make(map[string]float64)
	lastChunkSnapshot := make(map[string]time.Time)
	ticker := time.NewTicker(ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pa.ctx.Done():
			return
		case <-ticker.C:
		}

		if pa.pool == nil {
			continue
		}
		alpha := pa.getSpeedEmaAlpha()

		var batch []types.DownloadEvent
		activeConfigs := pa.pool.GetAll()

		for _, cfg := range activeConfigs {
			if cfg.ProgressState == nil || progress.CfgProgress(&cfg).IsPaused() || progress.CfgProgress(&cfg).Done.Load() {
				delete(lastSpeeds, cfg.ID)
				delete(lastChunkSnapshot, cfg.ID)
				continue
			}

			downloaded, total, totalElapsed, sessionElapsed, connections, sessionStart := progress.CfgProgress(&cfg).GetProgress()
			sessionDownloaded := downloaded - sessionStart

			var instantSpeed float64
			if sessionElapsed.Seconds() > 0 && sessionDownloaded > 0 {
				instantSpeed = float64(sessionDownloaded) / sessionElapsed.Seconds()
			}

			lastSpeed := lastSpeeds[cfg.ID]
			var currentSpeed float64
			if lastSpeed == 0 {
				currentSpeed = instantSpeed
			} else {
				currentSpeed = alpha*instantSpeed + (1-alpha)*lastSpeed
			}
			lastSpeeds[cfg.ID] = currentSpeed

			msg := types.DownloadEvent{
				Type:        types.EventProgress,
				DownloadID:  cfg.ID,
				Downloaded:  downloaded,
				Total:       total,
				Speed:       currentSpeed,
				Elapsed:     totalElapsed,
				Connections: int(connections),
				RateLimited: progress.CfgProgress(&cfg).RateLimited.Load(),
			}

			if time.Since(lastChunkSnapshot[cfg.ID]) >= 500*time.Millisecond {
				bitmap, width, _, chunkSize, chunkProgress := progress.CfgProgress(&cfg).GetBitmapSnapshot(true)
				if width > 0 && len(bitmap) > 0 {
					msg.ChunkBitmap = bitmap
					msg.BitmapWidth = width
					msg.ChunkSize = chunkSize
					msg.ChunkProgress = chunkProgress
					lastChunkSnapshot[cfg.ID] = time.Now()
				}
			}

			batch = append(batch, msg)
		}

		if len(batch) > 0 {
			_ = pa.eventBus.Publish(types.DownloadEvent{Type: types.EventBatchProgress, BatchEvents: batch})
		}
	}
}

func (pa *ProgressAggregator) Shutdown() {
	pa.cancel()
	pa.wg.Wait()
}
