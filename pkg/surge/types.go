package surge

import (
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

type (
	DownloadService = core.DownloadService

	LocalDownloadService  = core.LocalDownloadService
	RemoteDownloadService = core.RemoteDownloadService
	HTTPClientOptions     = core.HTTPClientOptions
	LifecycleHooks        = core.LifecycleHooks

	WorkerPool = download.WorkerPool

	LifecycleManager = processing.LifecycleManager
	EngineHooks      = processing.EngineHooks

	DownloadConfig = types.DownloadConfig
	RuntimeConfig  = types.RuntimeConfig
	DownloadStatus = types.DownloadStatus
	DownloadEntry  = types.DownloadEntry
	DownloadState  = types.DownloadState
	CancelResult   = types.CancelResult
	ProgressState  = types.ProgressState

	DownloadQueuedMsg   = events.DownloadQueuedMsg
	DownloadStartedMsg  = events.DownloadStartedMsg
	ProgressMsg         = events.ProgressMsg
	BatchProgressMsg    = events.BatchProgressMsg
	DownloadCompleteMsg = events.DownloadCompleteMsg
	DownloadErrorMsg    = events.DownloadErrorMsg
	DownloadPausedMsg   = events.DownloadPausedMsg
	DownloadResumedMsg  = events.DownloadResumedMsg
	DownloadRemovedMsg  = events.DownloadRemovedMsg
	DownloadRequestMsg  = events.DownloadRequestMsg
	SystemLogMsg        = events.SystemLogMsg
)
