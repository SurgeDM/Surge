package cmd

import "testing"

func TestCurrentRemoteClientConfigUsesAPIRequestTimeoutForResponseHeaders(t *testing.T) {
	config := currentRemoteClientConfig()
	if config.HTTPOptions.ResponseHeaderTimeout != defaultRemoteAPIRequestTimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", config.HTTPOptions.ResponseHeaderTimeout, defaultRemoteAPIRequestTimeout)
	}
}

func TestNewRemoteAPIHTTPClientRestrictsRedirects(t *testing.T) {
	client, err := newRemoteAPIHTTPClient()
	if err != nil {
		t.Fatalf("newRemoteAPIHTTPClient failed: %v", err)
	}
	if client.CheckRedirect == nil {
		t.Fatal("remote API client has no redirect policy")
	}
}
