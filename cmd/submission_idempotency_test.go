package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SurgeDM/Surge/internal/idempotency"
)

type idempotentTestService struct {
	*httpAPITestService
	calls atomic.Int32
}

func (s *idempotentTestService) AddWithID(_ string, _ string, _ string, _ []string, _ map[string]string, id string, _ bool, _ int, _ int64) (string, error) {
	s.calls.Add(1)
	return id, nil
}
func TestSubmissionReplayPreservesIdentity(t *testing.T) {
	setupIsolatedCmdState(t)
	previous := GlobalLifecycle
	GlobalLifecycle = nil
	t.Cleanup(func() { GlobalLifecycle = previous })
	service := &idempotentTestService{httpAPITestService: &httpAPITestService{}}
	ledger := idempotency.New(filepath.Join(t.TempDir(), "ledger.json"))
	submit := func(body, token string) (int, string) {
		r := httptest.NewRequest(http.MethodPost, "/download", strings.NewReader(body))
		r.Header.Set("Idempotency-Key", "artifact")
		r.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		handleDownload(w, r, t.TempDir(), service, ledger)
		var receipt submissionReceipt
		_ = json.Unmarshal(w.Body.Bytes(), &receipt)
		return w.Code, receipt.ID
	}
	// Use an explicit path so daemon destination changes are a real conflict.
	body := `{"url":"https://example.com/model","path":"/tmp/model","skip_approval":true}`
	code, id := submit(body, "Bearer one")
	if code != 200 || id == "" {
		t.Fatalf("first: %d %s", code, id)
	}
	code, replay := submit(body, "Bearer one")
	if code != 200 || replay != id || service.calls.Load() != 1 {
		t.Fatalf("replay created work: %d %s", code, replay)
	}
	if code, _ := submit(strings.ReplaceAll(body, "model", "other"), "Bearer one"); code != 409 {
		t.Fatalf("conflict=%d", code)
	}
	if code, other := submit(body, "Bearer two"); code != 200 || other == id || service.calls.Load() != 2 {
		t.Fatal("principals share identities")
	}
}
