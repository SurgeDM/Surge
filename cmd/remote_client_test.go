package cmd

import "testing"

func TestCurrentRemoteClientConfigUsesAPIRequestTimeoutForResponseHeaders(t *testing.T) {
	config := currentRemoteClientConfig()
	if config.HTTPOptions.ResponseHeaderTimeout != defaultRemoteAPIRequestTimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", config.HTTPOptions.ResponseHeaderTimeout, defaultRemoteAPIRequestTimeout)
	}
}
