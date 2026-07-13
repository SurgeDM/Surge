// lint:ignore-leak-check
package tui

import (
	"go.uber.org/goleak"
	"testing"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/SurgeDM/Surge/internal/orchestrator.(*EventBus).broadcastLoop"),
		goleak.IgnoreTopFunction("github.com/SurgeDM/Surge/internal/orchestrator.(*ProgressAggregator).reportProgressLoop"),
		goleak.IgnoreTopFunction("sync.runtime_notifyListWait"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
}
