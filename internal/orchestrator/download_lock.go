package orchestrator

import "sync"

type downloadLock struct {
	mu    sync.Mutex
	users int // Includes callers waiting to acquire mu.
}

// lockDownload orders admission/publication and removal for one ID. The map
// lock is held only for bookkeeping; slow publication cannot block other IDs.
// Reference counts keep one mutex alive for all waiters and discard it once
// the last caller leaves, without retaining every historical download ID.
func (mgr *LifecycleManager) lockDownload(id string) func() {
	mgr.downloadLocksMu.Lock()
	if mgr.downloadLocks == nil {
		mgr.downloadLocks = make(map[string]*downloadLock)
	}
	lock := mgr.downloadLocks[id]
	if lock == nil {
		lock = &downloadLock{}
		mgr.downloadLocks[id] = lock
	}
	lock.users++
	mgr.downloadLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		mgr.downloadLocksMu.Lock()
		lock.users--
		if lock.users == 0 {
			delete(mgr.downloadLocks, id)
		}
		mgr.downloadLocksMu.Unlock()
	}
}
