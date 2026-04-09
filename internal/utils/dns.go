package utils

import (
	"context"
	"net"
	"strings"
)

// ConfigureDialer modifies the provided net.Dialer to route all DNS lookups
// through the specified custom DNS server address.
// customAddr should include the port, e.g., "1.1.1.1:53".
func ConfigureDialer(dialer *net.Dialer, customAddr string) {
	if strings.TrimSpace(customAddr) == "" {
		return
	}

	// Ensure there is a port in the address. If not, default to 53.
	if _, _, err := net.SplitHostPort(customAddr); err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			customAddr = net.JoinHostPort(customAddr, "53")
		}
	}

	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Always route to the custom nameserver, ignoring the system address
			return dialer.DialContext(ctx, "udp", customAddr)
		},
	}
}
