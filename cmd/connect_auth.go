package cmd

import (
	"fmt"
	"os"
	"strings"
)

func resolveTokenForConnectTarget(target connectTarget) (string, error) {
	token := strings.TrimSpace(globalToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SURGE_TOKEN"))
	}
	if token != "" {
		return token, nil
	}

	serverHost, serverPort := parseRemoteServerAddress(target.BaseURL)
	if isLoopbackHost(serverHost) {
		details, ok := getActiveConnectionDetails()
		if !ok || details.port != serverPort {
			return "", fmt.Errorf("local target %q does not match the discovered Surge daemon: use --token or set SURGE_TOKEN", target.BaseURL)
		}
		return resolveLocalTokenForDetails(details)
	}
	return "", fmt.Errorf("remote target %q requires authentication: use --token or set SURGE_TOKEN", target.BaseURL)
}
