package concurrent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func openSnapshotTestFile(t *testing.T, destPath string) *os.File {
	t.Helper()
	file, err := os.Create(destPath + types.IncompleteSuffix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestHandlePause_Normal(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)
	cleanup := func() {}
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "test.bin")
	state := progress.New("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:      "test-id",
		State:   state,
		Runtime: &types.RuntimeConfig{},
	}

	remaining := []types.Task{{Offset: 500, Length: 500}}

	err := downloader.handlePause(openSnapshotTestFile(t, destPath), destPath, fileSize, remaining, nil)
	if !errors.Is(err, types.ErrPaused) {
		t.Fatalf("Expected ErrPaused, got %v", err)
	}
}

func TestHandlePause_PersistsWithoutLifecycleConsumer(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)
	destPath := filepath.Join(tmpDir, "test.bin")
	state := progress.New("test-id", 1000)
	progressCh := make(chan types.DownloadEvent, 1)
	progressCh <- types.DownloadEvent{}
	download := &ConcurrentDownloader{
		ID:           "test-id",
		URL:          "http://example.com/file.bin",
		State:        state,
		ProgressChan: progressCh,
		Runtime:      &types.RuntimeConfig{},
	}
	remaining := []types.Task{{Offset: 500, Length: 500}}

	if err := download.handlePause(openSnapshotTestFile(t, destPath), destPath, 1000, remaining, nil); !errors.Is(err, types.ErrPaused) {
		t.Fatalf("handlePause = %v, want ErrPaused", err)
	}
	if snapshot := state.TakePendingResumeState(); snapshot == nil {
		t.Fatal("pause state was not retained")
	}
}

func TestHandlePause_UsesLiveRateLimitFromState(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)
	cleanup := func() {}
	defer cleanup()

	fileSize := int64(1000)
	destPath := filepath.Join(tmpDir, "test.bin")
	state := progress.New("test-id", fileSize)
	state.SetRateLimit(3*1024*1024, true)
	progressCh := make(chan types.DownloadEvent, 1)
	downloader := &ConcurrentDownloader{
		ID:           "test-id",
		URL:          "http://example.com/file.bin",
		State:        state,
		ProgressChan: progressCh,
		Runtime:      &types.RuntimeConfig{},
		RateLimitBps: 1,
		RateLimitSet: false,
	}

	remaining := []types.Task{{Offset: 500, Length: 500}}

	err := downloader.handlePause(openSnapshotTestFile(t, destPath), destPath, fileSize, remaining, nil)
	if !errors.Is(err, types.ErrPaused) {
		t.Fatalf("Expected ErrPaused, got %v", err)
	}

	msg, ok := <-progressCh
	if !ok {
		t.Fatalf("expected DownloadPausedMsg, got %T", msg)
	}
	if msg.RateLimit != 3*1024*1024 || !msg.RateLimitSet {
		t.Fatalf("pause msg rate limit = (%d, %v), want (%d, true)", msg.RateLimit, msg.RateLimitSet, 3*1024*1024)
	}
	if msg.State == nil {
		t.Fatal("expected pause state")
	}
	if pending := state.TakePendingResumeState(); pending != nil {
		t.Fatalf("delivered pause event left fallback state pending: %+v", pending)
	}
}

func TestSetupTasks_NewDownloadClearsStaleProgress(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)
	cleanup := func() {}
	defer cleanup()

	fileSize := int64(1000)
	chunkSize := int64(500)
	destPath := filepath.Join(tmpDir, "new.bin")
	workingPath := destPath + types.IncompleteSuffix

	f, err := os.Create(workingPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	state := progress.New("test-id", fileSize)
	state.InitBitmap(fileSize, chunkSize)
	state.UpdateChunkStatus(0, 600, types.ChunkCompleted)
	state.Bytes.Downloaded.Store(600)
	downloader := &ConcurrentDownloader{
		ID:      "test-id",
		State:   state,
		Runtime: &types.RuntimeConfig{},
	}

	tasks, err := downloader.setupTasks(destPath, fileSize, chunkSize, 2, f, nil, false)
	if err != nil {
		t.Fatalf("setupTasks failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}
	if downloaded, verified := state.Bytes.Downloaded.Load(), state.Bytes.VerifiedProgress.Load(); downloaded != 0 || verified != 0 {
		t.Fatalf("fresh download retained byte counts: downloaded=%d, verified=%d", downloaded, verified)
	}
	_, _, _, _, chunkProgress := state.GetBitmapSnapshot(true)
	if !slices.Equal(chunkProgress, []int64{0, 0}) || len(state.Bitmap.GetCompletedRanges()) != 0 {
		t.Fatalf("fresh download retained chunk progress: %v", chunkProgress)
	}
	for i := range chunkProgress {
		if got := state.GetChunkState(i); got != types.ChunkPending {
			t.Fatalf("chunk %d has status %v, want pending", i, got)
		}
	}
}

func TestGetWorkerMirrors(t *testing.T) {
	d := &ConcurrentDownloader{URL: "http://primary.com"}
	active := []string{"http://primary.com", "http://mirror1.com", "http://mirror2.com"}

	mirrors := d.getWorkerMirrors(active)

	if len(mirrors) != 3 {
		t.Errorf("Expected 3 mirrors, got %d", len(mirrors))
	}
	if mirrors[0] != "http://primary.com" {
		t.Errorf("Primary URL should be first, got %s", mirrors[0])
	}
}

func TestInitMirrorStatus(t *testing.T) {
	state := progress.New("test-id", 1000)
	d := &ConcurrentDownloader{ID: "test-id", State: state}

	primary := "http://primary.com"
	candidates := []string{"http://mirror1.com", "http://mirror2.com"}
	active := []string{"http://primary.com", "http://mirror1.com"}

	d.initMirrorStatus(primary, candidates, active, "/path/to/dest")

	statuses := state.GetMirrors()
	if len(statuses) != 3 {
		t.Errorf("Expected 3 statuses, got %d", len(statuses))
	}

	foundMirror2 := false
	for _, s := range statuses {
		if s.URL == "http://mirror2.com" {
			foundMirror2 = true
			if s.Active {
				t.Error("Mirror2 should be inactive")
			}
			if !s.Error {
				t.Error("Mirror2 should have error (as it is not active)")
			}
		}
	}
	if !foundMirror2 {
		t.Error("Mirror2 status not found")
	}
}

func TestSetupTasks_BitmapRestoration(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)
	cleanup := func() {}
	defer cleanup()

	fileSize := int64(1000)
	chunkSize := int64(100)
	destPath := filepath.Join(tmpDir, "resume.bin")

	// Create a saved state
	savedBitmap := []byte{0xFF, 0x00, 0x00} // 10 chunks need 3 bytes
	savedState := &types.DownloadRecord{
		ID:              "test-id",
		URL:             "http://example.com",
		DestPath:        destPath,
		TotalSize:       fileSize,
		Downloaded:      500,
		ActualChunkSize: chunkSize,
		ChunkBitmap:     savedBitmap,
		Tasks:           []types.Task{{Offset: 500, Length: 500}},
	}
	if err := store.AddToMasterList(types.DownloadRecord{
		ID:         "test-id",
		URL:        "http://example.com",
		DestPath:   destPath,
		Filename:   filepath.Base(destPath),
		Status:     "paused",
		TotalSize:  fileSize,
		Downloaded: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState("http://example.com", destPath, savedState); err != nil {
		t.Fatal(err)
	}

	f, _ := os.Create(destPath + types.IncompleteSuffix)
	defer func() { _ = f.Close() }()

	progState := progress.New("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:    "test-id",
		URL:   "http://example.com",
		State: progState,
	}

	// This simulates the fixed order in Download():
	// 1. InitBitmap
	progState.InitBitmap(fileSize, chunkSize)
	// 2. setupTasks (which calls RestoreBitmap)
	_, err := downloader.setupTasks(destPath, fileSize, chunkSize, 1, f, savedState, true)
	if err != nil {
		t.Fatal(err)
	}

	// Verify bitmap is NOT empty (it should have the restored data)
	bitmap, _, _, _, _ := progState.GetBitmapSnapshot(false)
	if len(bitmap) == 0 {
		t.Error("Bitmap should have been restored, but it is empty")
	}
	if bitmap[0] != 0xAA {
		t.Errorf("Bitmap[0] should be 0xAA (all chunks completed), got 0x%02X", bitmap[0])
	}
}

func TestSaveStateSnapshot_PersistsReturnedAndStolenRangesOnce(t *testing.T) {
	tmpDir := testutil.SetupStateDB(t)

	const fileSize = 16 * types.AlignSize
	const downloaded = 4 * types.AlignSize
	destPath := filepath.Join(tmpDir, "snapshot.bin")
	state := progress.New("test-id", fileSize)
	downloader := &ConcurrentDownloader{
		ID:          "test-id",
		State:       state,
		Runtime:     &types.RuntimeConfig{MinChunkSize: types.AlignSize},
		URL:         "http://example.com/snapshot.bin",
		activeTasks: make(map[int]*ActiveTask),
	}

	if err := store.AddToMasterList(types.DownloadRecord{
		ID:        "test-id",
		URL:       "http://example.com/snapshot.bin",
		DestPath:  destPath,
		Status:    "downloading",
		TotalSize: fileSize,
	}); err != nil {
		t.Fatal(err)
	}

	queue := NewTaskQueue()
	active := &ActiveTask{Task: types.Task{Length: fileSize}}
	active.CurrentOffset.Store(downloaded)
	active.StopAt.Store(fileSize)
	downloader.activeTasks[0] = active
	state.ActiveWorkers.Add(1)
	if !downloader.StealWork(queue) {
		t.Fatal("balancer did not split the active task")
	}
	if downloader.finishTask(0, active, queue, fileSize, true) {
		t.Fatal("unfinished task reported complete")
	}
	if len(downloader.activeTasks) != 0 || state.ActiveWorkers.Load() != 0 {
		t.Fatal("worker cleanup left the task registered")
	}
	if task, ok := queue.Pop(); ok {
		t.Fatalf("worker received a task after dispatch stopped: %+v", task)
	}

	err := downloader.saveStateSnapshot(openSnapshotTestFile(t, destPath), destPath, fileSize, queue.DrainRemaining(), nil, false)
	if err != nil {
		t.Fatalf("saveStateSnapshot failed: %v", err)
	}

	// Load the saved state to verify
	saved, err := store.LoadState(downloader.URL, destPath)
	if err != nil {
		t.Fatalf("failed to load saved state: %v", err)
	}

	want := []types.Task{
		{Offset: 10 * types.AlignSize, Length: 6 * types.AlignSize},
		{Offset: downloaded, Length: 6 * types.AlignSize},
	}
	if !slices.Equal(saved.Tasks, want) {
		t.Errorf("saved tasks = %+v, want %+v", saved.Tasks, want)
	}
	if saved.Downloaded != downloaded {
		t.Errorf("saved downloaded bytes = %d, want %d", saved.Downloaded, downloaded)
	}
}

func TestResumeRebuildsBitmapForCurrentConnectionLimit(t *testing.T) {
	const size = int64(128 * 1024 * 1024)
	for _, limits := range [][2]int{{1, 8}, {8, 1}} {
		t.Run(fmt.Sprintf("%d_to_%d", limits[0], limits[1]), func(t *testing.T) {
			oldRuntime := &types.RuntimeConfig{MaxConnectionsPerDownload: limits[0]}
			state := progress.New("resume-map", size)
			d := &ConcurrentDownloader{State: state, Runtime: oldRuntime}
			oldChunkSize := d.determineChunkSize(size, d.getInitialConnections(size))
			state.InitBitmap(size, oldChunkSize)
			bitmap, _, _, _, _ := state.GetBitmapSnapshot(false)
			saved := &types.DownloadRecord{
				TotalSize: size, Downloaded: size / 4,
				ActualChunkSize: oldChunkSize, ChunkBitmap: bitmap,
				Tasks: []types.Task{{Offset: size / 4, Length: size * 3 / 4}},
			}
			d.Runtime = &types.RuntimeConfig{MaxConnectionsPerDownload: limits[1]}
			conns := d.getInitialConnections(d.getEffectiveSizeForWorkers(size, saved, true))
			chunkSize := d.determineChunkSize(size, conns)
			state.InitBitmap(size, chunkSize)
			tasks, err := d.setupTasks("", size, chunkSize, conns, nil, saved, true)
			if err != nil {
				t.Fatal(err)
			}
			_, width, _, actualChunkSize, _ := state.GetBitmapSnapshot(false)
			if actualChunkSize != chunkSize || width != int((size-1)/chunkSize+1) {
				t.Fatalf("layout = %d chunks of %d bytes, want chunk size %d", width, actualChunkSize, chunkSize)
			}
			if got := state.Bytes.VerifiedProgress.Load(); got != size/4 {
				t.Fatalf("verified progress = %d, want %d", got, size/4)
			}
			if len(tasks) != 1 || tasks[0] != saved.Tasks[0] {
				t.Fatalf("remaining ranges changed: %v", tasks)
			}
		})
	}
}

func TestResumeChangedLayoutPreservesCompletedBitmapRanges(t *testing.T) {
	const size int64 = 100
	state := progress.New("resume-overlap", size)
	state.InitBitmap(size, 25)
	d := &ConcurrentDownloader{State: state}
	saved := &types.DownloadRecord{TotalSize: size, ActualChunkSize: 20, ChunkBitmap: []byte{byte(types.ChunkCompleted)}, Tasks: []types.Task{{Offset: 0, Length: 100}}}
	tasks, err := d.setupTasks("", size, 25, 1, nil, saved, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0] != (types.Task{Offset: 20, Length: 80}) {
		t.Fatalf("remaining tasks = %v", tasks)
	}
	if got := state.Bytes.VerifiedProgress.Load(); got != 20 {
		t.Fatalf("verified progress = %d, want 20", got)
	}
}
