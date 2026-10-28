package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

const maxManifestBytes = 1 << 20

type downloadManifest struct {
	SchemaVersion int            `json:"schema_version"`
	Items         []manifestItem `json:"items"`
}
type manifestItem struct {
	ClientRef string   `json:"client_ref"`
	URL       string   `json:"url"`
	Mirrors   []string `json:"mirrors,omitempty"`
	Output    string   `json:"output,omitempty"`
	Filename  string   `json:"filename,omitempty"`
	SHA256    string   `json:"sha256,omitempty"`
	OnExists  string   `json:"on_exists,omitempty"`
}

func configureManifest(command *cobra.Command) {
	command.Flags().String("manifest", "", "Version 1 JSON manifest file, or - for stdin (max 1 MiB, 256 items)")
	command.Flags().Bool("allow-partial", false, "Submit valid manifest items even when another item is invalid")
	command.Flags().String("idempotency-key", "", "Durable remote submission namespace (1-128 printable ASCII characters)")
}
func boundedInput(cmd *cobra.Command, path string) ([]byte, error) {
	reader := cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, usageError("Cannot open input file.")
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil || len(data) > maxManifestBytes {
		return nil, usageError("Input exceeds 1 MiB or could not be read.")
	}
	return data, nil
}
func readManifest(cmd *cobra.Command, path string, partial bool) ([]DownloadRequest, error) {
	data, err := boundedInput(cmd, path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest downloadManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, usageError("Invalid manifest JSON or unsupported fields.")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, usageError("A manifest must contain exactly one JSON object.")
	}
	if manifest.SchemaVersion != 1 || len(manifest.Items) == 0 || len(manifest.Items) > maxWaitIDs {
		return nil, usageError("A version 1 manifest requires 1-256 items.")
	}
	refs := make(map[string]bool, len(manifest.Items))
	requests := make([]DownloadRequest, 0, len(manifest.Items))
	for _, item := range manifest.Items {
		// Correlation failures invalidate the document even in partial mode.
		if item.ClientRef == "" || len(item.ClientRef) > 128 || strings.IndexFunc(item.ClientRef, unicode.IsControl) >= 0 || refs[item.ClientRef] {
			return nil, usageError("Manifest client_ref values must be unique, nonempty, and at most 128 bytes without controls.")
		}
		refs[item.ClientRef] = true
		policy := item.OnExists
		if policy == "" {
			policy = "fail"
		}
		request, validationErr := validateDownloadRequest(DownloadRequest{URL: item.URL, Mirrors: item.Mirrors, Path: item.Output, Filename: item.Filename, Artifact: types.ArtifactPolicy{OnExists: policy, ExpectedSHA256: item.SHA256}, SkipApproval: true, SkipDuplicateWarning: true, IsExplicitCategory: true})
		if validationErr == nil {
			normalized, urlErr := ValidateAndNormalizeURL(item.URL)
			if urlErr != nil {
				validationErr = urlErr
			} else {
				request.URL = normalized
			}
			for index, target := range item.Mirrors {
				normalized, urlErr := ValidateAndNormalizeURL(target)
				if urlErr != nil {
					validationErr = urlErr
					break
				}
				request.Mirrors[index] = normalized
			}
		}
		request.ClientRef = item.ClientRef
		if validationErr != nil {
			if !partial {
				return nil, usageError("Manifest contains an invalid URL, filename, destination policy, or SHA-256; no work was submitted.")
			}
			failure := clioutput.Describe(usageError("Invalid manifest download input."))
			request.inputFailure = &failure
		}
		requests = append(requests, request)
	}
	return requests, nil
}
func readBatchInputs(cmd *cobra.Command, path string) ([]string, error) {
	data, err := boundedInput(cmd, path)
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
		if len(urls) > maxWaitIDs {
			return nil, usageError("Batch input exceeds 256 URLs.")
		}
	}
	return urls, nil
}
func manifestRequests(cmd *cobra.Command, args []string) ([]DownloadRequest, bool, error) {
	path, _ := cmd.Flags().GetString("manifest")
	partial, _ := cmd.Flags().GetBool("allow-partial")
	key, _ := cmd.Flags().GetString("idempotency-key")
	if key != "" && !validIdempotencyKey(key) {
		return nil, false, usageError("--idempotency-key requires 1-128 printable ASCII characters.")
	}
	if path == "" {
		if partial {
			return nil, false, usageError("--allow-partial requires --manifest.")
		}
		return nil, false, nil
	}
	if len(args) > 0 {
		return nil, true, usageError("--manifest cannot be combined with URL arguments.")
	}
	for _, name := range []string{"batch", "output", "filename", "sha256", "on-exists", "resume", "confirm"} {
		if cmd.Flags().Changed(name) {
			return nil, true, usageError("--manifest cannot be combined with per-command input or destination overrides.")
		}
	}
	requests, err := readManifest(cmd, path, partial)
	return requests, true, err
}
