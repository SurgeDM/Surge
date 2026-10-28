package cmd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/SurgeDM/Surge/internal/clioutput"
)

type serverCapabilities struct {
	SchemaVersion int             `json:"schema_version"`
	Features      map[string]bool `json:"features"`
}

func readServerCapabilities(ctx context.Context, client *http.Client, baseURL, token string) (serverCapabilities, error) {
	resp, err := commandAPIRequest(ctx, client, http.MethodGet, baseURL, token, "/capabilities")
	if err != nil {
		return serverCapabilities{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotImplemented {
		return serverCapabilities{}, clioutput.NewError(clioutput.Conflict, "Upgrade the daemon to a version supporting capabilities.", nil)
	}
	if resp.StatusCode != http.StatusOK {
		return serverCapabilities{}, remoteStatusError(resp.StatusCode, fmt.Errorf("server capabilities unavailable: HTTP %d", resp.StatusCode))
	}
	var result serverCapabilities
	if err := decodeCommandResponse(resp.Body, &result); err != nil {
		return result, err
	}
	if result.SchemaVersion != 1 {
		return result, clioutput.NewError(clioutput.Conflict, "The server uses an unsupported capabilities version.", nil)
	}
	return result, nil
}
