// lint:ignore-leak-check
package scheduler

import (
	"go.uber.org/goleak"
	"testing"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("sync.runtime_notifyListWait"),
		goleak.IgnoreTopFunction("github.com/SurgeDM/Surge/internal/scheduler.safeSendProgress"),
	)
}
