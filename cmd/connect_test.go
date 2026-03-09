package cmd

import "testing"

func TestNewRemoteRootModel_UsesNilOrchestrator(t *testing.T) {
	m := newRemoteRootModel(1700, nil, "example.com")

	if m.Orchestrator != nil {
		t.Fatal("expected remote root model to use nil orchestrator")
	}
	if !m.IsRemote {
		t.Fatal("expected remote root model to be marked remote")
	}
	if m.ServerHost != "example.com" {
		t.Fatalf("server host = %q, want example.com", m.ServerHost)
	}
}
