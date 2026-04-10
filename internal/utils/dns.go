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
	host, port, err := net.SplitHostPort(customAddr)
	if err != nil {
		host = customAddr
		port = "53"
	}
	customAddr = net.JoinHostPort(host, port)

	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Use a clean dialer with no custom resolver to avoid recursive resolution
			// when customAddr is a hostname rather than a literal IP.
			d := net.Dialer{Timeout: dialer.Timeout}
			return d.DialContext(ctx, "udp", customAddr)
		},
	}
}
