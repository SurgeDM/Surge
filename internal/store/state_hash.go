package store

import (
	"context"
	"os"
	"reflect"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

// No queue and at most two payload readers, even across Configure/CloseDB.
// A stuck OS read consumes a slot rather than spawning unbounded replacements.
// Hashes are optional: saturation and shutdown leave the saved checkpoint usable.
var stateHashSlots = make(chan struct{}, 2)

// Protected by masterMu. Each job owns a detached checkpoint, never a caller's
// DownloadRecord. Identity checks prevent canceled jobs from removing successors.
var stateHashJobs = make(map[string]*stateHashJob)

type stateHashJob struct {
	cancel context.CancelFunc
}

// Captured at admission; injectable for deterministic slow-filesystem tests.
var hashFileForState = computeStableFileHashMD5

func cancelStateHashLocked(id string) {
	if job := stateHashJobs[id]; job != nil {
		job.cancel()
		delete(stateHashJobs, id)
	}
}

func cancelAllStateHashesLocked() {
	for id := range stateHashJobs {
		cancelStateHashLocked(id)
	}
}

// Called only after the checkpoint and index writes have succeeded, with
// masterMu held. Admission never waits for hashing capacity.
func startStateHashLocked(dir, detailPath string, snapshot DetailState, timeout time.Duration) {
	select {
	case stateHashSlots <- struct{}{}:
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	job := &stateHashJob{cancel: cancel}
	id := snapshot.State.ID
	stateHashJobs[id] = job
	hashFile := hashFileForState
	go func() {
		defer func() {
			cancel()
			masterMu.Lock()
			if stateHashJobs[id] == job {
				delete(stateHashJobs, id)
			}
			masterMu.Unlock()
			<-stateHashSlots
		}()
		hash, timedOut, err := hashFile(ctx, snapshot.State.DestPath+types.IncompleteSuffix, timeout)
		if err != nil || timedOut || hash == "" || ctx.Err() != nil {
			utils.Debug("State hash skipped for %s: timeout=%v error=%v", id, timedOut, err)
			return
		}

		masterMu.Lock()
		defer masterMu.Unlock()
		if ctx.Err() != nil || stateHashJobs[id] != job || baseDir != dir || movingDownloads[id] {
			return
		}
		var current DetailState
		if err := loadGob(detailPath, &current); err != nil || current.Revision != snapshot.Revision ||
			!reflect.DeepEqual(current.State, snapshot.State) {
			return
		}
		// Patch only the hash in the current detail; never replay the index or
		// recreate deleted state. Failure leaves the original checkpoint intact.
		current.State.FileHash = hash
		if err := atomicWrite(detailPath, current); err != nil {
			utils.Debug("State hash update failed for %s: %v", id, err)
		}
	}()
}

func sameHashFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}
