package runtime

type RuntimeSnapshot struct {
	SettingsLoaded bool
	ServiceReady   bool
	LifecycleReady bool
	ActiveCount    int
	DownloadCount  int
}

func (a *App) Snapshot() RuntimeSnapshot {
	snapshot := RuntimeSnapshot{}
	if a == nil {
		return snapshot
	}

	snapshot.SettingsLoaded = a.settings != nil
	snapshot.ServiceReady = a.service != nil
	snapshot.LifecycleReady = a.CurrentLifecycle() != nil

	if a.pool != nil {
		snapshot.ActiveCount = a.pool.ActiveCount()
		snapshot.DownloadCount = len(a.pool.GetAll())
	}

	return snapshot
}
