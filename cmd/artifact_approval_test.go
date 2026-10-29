package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestApprovalCannotDropArtifactPolicy(t *testing.T) {
	setupIsolatedCmdState(t)
	previous := serverProgram
	serverProgram = &tea.Program{}
	t.Cleanup(func() { serverProgram = previous })
	settings := config.DefaultSettings()
	settings.Extension.ExtensionPrompt.Value = true
	service := &publishRecordingHTTPService{httpAPITestService: &httpAPITestService{}}
	resolved := &resolvedDownloadRequest{settings: settings, request: DownloadRequest{URL: "https://example.com/model", Artifact: types.ArtifactPolicy{OnExists: "fail", ExpectedSHA256: strings.Repeat("a", 64)}}}
	recorder := httptest.NewRecorder()
	if !maybeRequireDownloadApproval(recorder, service, resolved) || recorder.Code != http.StatusConflict || len(service.published) != 0 {
		t.Fatalf("unsafe approval: status=%d events=%+v", recorder.Code, service.published)
	}
	resolved.request.SkipApproval = true
	if maybeRequireDownloadApproval(httptest.NewRecorder(), service, resolved) {
		t.Fatal("immediate artifact request incorrectly requires approval")
	}
}
func TestBatchCannotDropArtifactPolicy(t *testing.T) {
	setupIsolatedCmdState(t)
	previous := serverProgram
	serverProgram = &tea.Program{}
	t.Cleanup(func() { serverProgram = previous })
	for _, immediate := range []bool{false, true} {
		service := &batchAddRecordingService{httpAPITestService: &httpAPITestService{}}
		approval := "false"
		if immediate {
			approval = "true"
		}
		request := httptest.NewRequest(http.MethodPost, "/download/batch", strings.NewReader(`{"skip_approval":`+approval+`,"downloads":[{"url":"https://example.com/model","artifact":{"on_exists":"fail"}}]}`))
		recorder := httptest.NewRecorder()
		handleBatchDownload(recorder, request, t.TempDir(), service)
		// This service has no artifact support. It must never receive an unprotected Add.
		if recorder.Code == http.StatusOK || recorder.Code == http.StatusAccepted || len(service.added) != 0 {
			t.Fatalf("batch discarded its policy: immediate=%t status=%d added=%v", immediate, recorder.Code, service.added)
		}
		if !immediate && recorder.Code != http.StatusConflict {
			t.Fatalf("approval status=%d", recorder.Code)
		}
	}
}
