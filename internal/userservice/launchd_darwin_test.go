//go:build darwin

package userservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type launchdStatusRunner struct{ output string }

func (r launchdStatusRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return []byte(r.output), nil
}
func TestLaunchdStatusDistinguishesLoadedFromRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.plist")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		output string
		want   State
	}{
		{"gui/1000/com.surgedm.surge = {\n\tstate = not running\n}", Stopped},
		{"gui/1000/com.surgedm.surge = {\n\tstate = running\n\tpid = 123\n}", Running},
	} {
		m := &LaunchdManager{plistPath: path, uid: 1000, runner: launchdStatusRunner{test.output}}
		got, err := m.Status(context.Background())
		if err != nil || got != test.want {
			t.Fatalf("Status = %v, %v; want %v", got, err, test.want)
		}
	}
}
