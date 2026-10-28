package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/google/uuid"
)

type agentEventTestService struct {
	*httpAPITestService
	stream     chan types.DownloadEvent
	subscribed bool
}

func (s *agentEventTestService) StreamObserver(_ context.Context, _ types.ObserverOptions) (<-chan types.DownloadEvent, func(), error) {
	s.subscribed = true
	return s.stream, func() {}, nil
}
func (s *agentEventTestService) GetStatus(id string) (*types.DownloadStatus, error) {
	if !s.subscribed {
		panic("snapshot before subscribe")
	}
	return &types.DownloadStatus{ID: id, Status: "completed", Filename: "model", URL: "https://user:secret@example.com"}, nil
}
func TestAgentStreamStartsWithSnapshotAndInvalidates(t *testing.T) {
	id := uuid.NewString()
	stream := make(chan types.DownloadEvent, 1)
	stream <- types.DownloadEvent{Type: types.EventStarted, DownloadID: id, URL: "https://secret", Headers: map[string]string{"Authorization": "secret"}}
	close(stream)
	service := &agentEventTestService{httpAPITestService: &httpAPITestService{}, stream: stream}
	server := testutil.NewHTTPServerT(t, agentEventsHandler(service))
	defer server.Close()
	response, err := http.Get(server.URL + "?ids=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	decoder := json.NewDecoder(response.Body)
	var snapshot, notice, resync agentEvent
	if err := decoder.Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&notice); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&resync); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "snapshot" || snapshot.Items[0].Status != "completed" || notice.Type != "state_changed" || notice.ID != id || resync.Type != "resync_required" {
		t.Fatalf("unexpected stream: %+v %+v %+v", snapshot, notice, resync)
	}
	data, _ := json.Marshal([]agentEvent{snapshot, notice, resync})
	if strings.Contains(string(data), "secret") {
		t.Fatal("credentials leaked")
	}
	if notice.Sequence != snapshot.Sequence+1 || notice.Epoch != snapshot.Epoch {
		t.Fatal("stream sequence changed epoch")
	}
}

func TestEventsCommandWritesOneRecordPerLine(t *testing.T) {
	resetCommandConnectionState(t)
	id := uuid.NewString()
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(agentEvent{SchemaVersion: 1, Type: "snapshot", Epoch: "epoch", Sequence: 1})
		_ = encoder.Encode(agentEvent{SchemaVersion: 1, Type: "resync_required", Epoch: "epoch", Sequence: 2})
	}))
	defer server.Close()
	root := newOutputTestRoot(t)
	globalHost, globalToken = server.URL, "token"
	root.AddCommand(newEventsCommand())
	var out, diagnostic strings.Builder
	if code := runCommand(root, []string{"events", id}, &out, &diagnostic); code != 6 {
		t.Fatalf("code=%d output=%s errors=%s", code, out.String(), diagnostic.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("not NDJSON: %s", out.String())
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid record: %q", line)
		}
	}
}
