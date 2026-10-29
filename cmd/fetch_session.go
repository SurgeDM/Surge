package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/google/uuid"
)

type fetchJob struct {
	ID         string `json:"id"`
	InputIndex int    `json:"input_index"`
	ClientRef  string `json:"client_ref,omitempty"`
}
type fetchSession struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	Jobs          []fetchJob `json:"jobs"`
}
type fetchOwner struct {
	finalErrors      map[string]error
	finalStatuses    map[string]*types.DownloadStatus
	checkpointsValid bool
	manager          *orchestrator.LifecycleManager
	service          *service.LocalDownloadService
	lock             *InstanceLock
	session          fetchSession
	workerDone       <-chan struct{}
	previousState    string
}

func newFetchOwner(resume string, count int) (*fetchOwner, error) {
	session := fetchSession{SchemaVersion: 1, ID: uuid.NewString()}
	if resume != "" {
		if _, err := uuid.Parse(resume); err != nil {
			return nil, clioutput.NewError(clioutput.InvalidArguments, "Resume requires a full fetch session ID.", nil)
		}
		session.ID = resume
	}
	dir := filepath.Join(config.GetStateDir(), "foreground", session.ID)
	if resume != "" {
		data, err := os.ReadFile(filepath.Join(dir, "session.json"))
		if os.IsNotExist(err) {
			return nil, clioutput.NewError(clioutput.NotFound, "Fetch session was not found.", nil)
		}
		if err != nil {
			return nil, err
		}
		if len(data) > 128*1024 || json.Unmarshal(data, &session) != nil || session.SchemaVersion != 1 || session.ID != resume || len(session.Jobs) == 0 || len(session.Jobs) > maxWaitIDs {
			return nil, fmt.Errorf("invalid saved fetch session")
		}
	}
	seen := make(map[string]bool, len(session.Jobs))
	for _, job := range session.Jobs {
		if _, err := uuid.Parse(job.ID); err != nil || seen[job.ID] || job.InputIndex < 1 || job.InputIndex > maxWaitIDs {
			return nil, fmt.Errorf("invalid saved fetch job")
		}
		seen[job.ID] = true
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, acquired, err := TryAcquireInstanceLock(dir)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, clioutput.NewError(clioutput.Conflict, "Another process owns this fetch session.", nil)
	}
	success := false
	defer func() {
		if !success {
			_ = lock.Release()
		}
	}()
	if resume == "" {
		for i := range count {
			session.Jobs = append(session.Jobs, fetchJob{ID: uuid.NewString(), InputIndex: i + 1})
		}
		data, err := json.Marshal(session)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, "session.json"), data, 0600); err != nil {
			return nil, err
		}
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return nil, err
	}
	settings.General.DownloadCompleteNotification.Value = false
	previous := store.StateDirectory()
	store.Configure(filepath.Join(dir, "surge.db"))
	bus := orchestrator.NewEventBus()
	pool := scheduler.New(bus.InputCh, config.Resolve[int](settings.Network.MaxConcurrentDownloads))
	mgr := orchestrator.NewLifecycleManager(pool, bus, settings)
	stream, _ := bus.SubscribeLifecycle()
	done := make(chan struct{})
	go func() { defer close(done); mgr.StartEventWorker(stream) }()
	success = true
	return &fetchOwner{manager: mgr, service: service.NewLocalDownloadService(mgr), lock: lock, session: session, workerDone: done, previousState: previous}, nil
}

// Never restore the global store while workers might still be writing to it.
// On a forced/bounded exit the process boundary exits and the next invocation
// validates the last saved checkpoint; no final checkpoint is promised.
func (o *fetchOwner) close(force <-chan struct{}) bool {
	done := make(chan struct{})
	go func() {
		o.manager.Shutdown()
		<-o.workerDone
		o.finalStatuses = make(map[string]*types.DownloadStatus)
		o.finalErrors = make(map[string]error)
		o.checkpointsValid = true
		for _, job := range o.session.Jobs {
			status, err := o.service.GetResult(job.ID)
			if err != nil || status == nil {
				if err == nil {
					err = clioutput.NewError(clioutput.OperationFailed, "Final download status is unavailable.", nil)
				}
				o.finalErrors[job.ID] = err
				o.checkpointsValid = false
				continue
			}
			o.finalStatuses[job.ID] = status
			if status.Status == "queued" || status.Status == "completed" {
				continue
			}
			if status.Status != "paused" {
				o.checkpointsValid = false
				continue
			}
			record, readErr := store.GetDownload(job.ID)
			if readErr != nil || record == nil {
				o.checkpointsValid = false
				continue
			}
			saved, readErr := store.LoadState(record.URL, record.DestPath)
			if readErr != nil || saved == nil ||
				saved.ID != record.ID || saved.URL != record.URL || saved.DestPath != record.DestPath ||
				saved.Downloaded != record.Downloaded || saved.TotalSize != record.TotalSize ||
				saved.Elapsed/int64(time.Millisecond) != record.TimeTaken {
				o.checkpointsValid = false
			}
		}
		close(done)
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		if o.previousState == "" {
			store.CloseDB()
		} else {
			store.Configure(filepath.Join(o.previousState, "surge.db"))
		}
		_ = o.lock.Release()
		return true
	case <-force:
		return false
	case <-timer.C:
		return false
	}
}

func (o *fetchOwner) resume(ctx context.Context) error {
	if _, err := store.NormalizeStaleDownloads(); err != nil {
		return err
	}
	for _, job := range o.session.Jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := store.GetDownload(job.ID)
		if err != nil {
			return err
		}
		if record == nil {
			continue
		}
		if record.Status == "completed" {
			continue
		}
		if record.ErrorCode == "integrity_mismatch" {
			return clioutput.NewError(clioutput.Conflict, "A saved artifact failed verification; inspect it or start a new session with a new destination.", nil)
		}
		if err := o.service.Resume(job.ID); err != nil {
			return err
		}
	}
	return nil
}
