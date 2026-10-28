package types

import (
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrDestinationExists = errors.New("destination already exists; choose a new destination or resume the saved session")
	ErrIntegrityMismatch = errors.New("artifact SHA-256 does not match the expected digest; working file retained")
)

// ArtifactPolicy is optional. Its zero value retains the existing engine policy.
// ExpectedSHA256 never changes during a job; SHA256/Verification describe the
// completed verification pass, not a hash of partially downloaded bytes.
type ArtifactPolicy struct {
	OnExists       string `json:"on_exists,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	Verification   string `json:"verification,omitempty"`
}

func (p ArtifactPolicy) Validate() error {
	if p.SHA256 != "" || p.Verification != "" {
		return errors.New("computed digest and verification fields cannot be supplied by a caller")
	}
	if p.OnExists != "" && p.OnExists != "fail" && p.OnExists != "rename" {
		return errors.New("on_exists must be fail or rename")
	}
	if p.ExpectedSHA256 != "" {
		digest, err := hex.DecodeString(p.ExpectedSHA256)
		if err != nil || len(digest) != 32 {
			return errors.New("expected SHA-256 must contain exactly 64 hexadecimal characters")
		}
	}
	return nil
}
func (p ArtifactPolicy) Normalized() ArtifactPolicy {
	p.ExpectedSHA256 = strings.ToLower(p.ExpectedSHA256)
	return p
}
