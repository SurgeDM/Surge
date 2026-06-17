package utils

import (
	"context"
	"net"
	"strings"
)

// normalizeDNSAddr turns the custom DNS setting into a single dial target
// (host:port). The setting accepts a comma-separated list (see ValidateDNSList
// and the in-app help text), so only the first server is used here; a missing
// port defaults to 53.
func normalizeDNSAddr(customAddr string) string {
	// Use the first non-empty server of a possibly comma-separated list. Without
	// this the whole list would be treated as one host:port and net.SplitHostPort
	// would fail, producing an invalid dial target like "[1.1.1.1:53, 8.8.8.8:53]:53"
	// that breaks every DNS lookup. Skipping empty entries also handles stray
	// commas/whitespace (e.g. ", 8.8.8.8:53") gracefully; an empty result means
	// no usable server was provided.
	first := ""
	for _, part := range strings.Split(customAddr, ",") {
		if p := strings.TrimSpace(part); p != "" {
			first = p
			break
		}
	}
	if first == "" {
		return ""
	}

	// Ensure there is a port in the address. If not, default to 53.
	host, port, err := net.SplitHostPort(first)
	if err != nil {
		host = first
		port = "53"
	}
	return net.JoinHostPort(host, port)
}

// ConfigureDialer modifies the provided net.Dialer to route all DNS lookups
// through the specified custom DNS server address.
// customAddr should include the port, e.g., "1.1.1.1:53".
func ConfigureDialer(dialer *net.Dialer, customAddr string) {
	if strings.TrimSpace(customAddr) == "" {
		return
	}

	target := normalizeDNSAddr(customAddr)
	if target == "" {
		// The value was only separators/whitespace: no usable server, so leave
		// the dialer's default resolver in place rather than installing a broken one.
		return
	}

	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Use a clean dialer with no custom resolver to avoid recursive resolution
			// when customAddr is a hostname rather than a literal IP.
			d := net.Dialer{Timeout: dialer.Timeout}
			return d.DialContext(ctx, "udp", target)
		},
	}
}
