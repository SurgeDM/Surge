package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestFinalArtifactPolicyNeverOverwritesRacingDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := finalizeArtifact(path, types.ArtifactPolicy{OnExists: "fail"}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("racing destination accepted: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "existing bytes" {
		t.Fatal("destination overwritten")
	}
	data, _ = os.ReadFile(path + types.IncompleteSuffix)
	if string(data) != "new bytes" {
		t.Fatal("working bytes discarded")
	}
}

func TestArtifactPublicationRequiresProofOfRequestedVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := finalizeArtifact(path, types.ArtifactPolicy{ExpectedSHA256: "expected"}); !errors.Is(err, types.ErrIntegrityMismatch) {
		t.Fatalf("unverified artifact accepted: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unverified artifact published")
	}
}
