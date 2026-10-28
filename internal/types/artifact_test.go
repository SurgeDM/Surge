package types

import (
	"strings"
	"testing"
)

func TestArtifactPolicyRejectsForgedVerification(t *testing.T) {
	for _, policy := range []ArtifactPolicy{{SHA256: strings.Repeat("0", 64)}, {Verification: "verified"}, {ExpectedSHA256: "short"}, {OnExists: "overwrite"}} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("unsafe artifact policy accepted: %+v", policy)
		}
	}
	policy := ArtifactPolicy{OnExists: "fail", ExpectedSHA256: strings.Repeat("A", 64)}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := policy.Normalized().ExpectedSHA256; got != strings.Repeat("a", 64) {
		t.Fatal("digest was not canonicalized")
	}
}
