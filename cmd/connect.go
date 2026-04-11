package cmd

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/tui"
	"github.com/spf13/cobra"
)

type connectTarget struct {
	BaseURL string
}

var connectCmd = &cobra.Command{
	Use:   "connect [host:port]",
	Short: "Connect TUI to a running Surge daemon",
	Long:  `Connect to a running Surge daemon and open the TUI. When no target is specified, auto-detects a locally running server.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		hostTarget := resolveHostTarget()
		if len(args) > 0 {
			target = args[0]
		} else if hostTarget != "" {
			target = hostTarget
		} else {
			port := readActivePort()
			if port == 0 {
				return fmt.Errorf("no local Surge server detected. Start one with 'surge' or 'surge server', or specify a target: surge connect <host:port>")
			}
			target = fmt.Sprintf("127.0.0.1:%d", port)
			fmt.Fprintf(os.Stderr, "Auto-detected local server on port %d\n", port)
		}
		return connectAndRunTUI(cmd, target)
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func connectAndRunTUI(_ *cobra.Command, target string) error {
	clientCfg := currentRemoteClientConfig()
	parsed, err := parseConnectTarget(target, clientCfg.AllowInsecureHTTP)
	if err != nil {
		return err
	}

	token, err := resolveTokenForConnectTarget(parsed)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Connecting to %s...\n", parsed.BaseURL)

	service, err := newRemoteDownloadService(parsed.BaseURL, token)
	if err != nil {
		return fmt.Errorf("failed to configure remote client: %w", err)
	}
	defer func() { _ = service.Shutdown() }()

	requestTimeout := service.Client.Timeout
	service.Client.Timeout = clientCfg.ConnectTimeout
	_, err = service.List()
	service.Client.Timeout = requestTimeout
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	stream, cleanup, err := service.StreamEvents(streamCtx)
	if err != nil {
		return fmt.Errorf("failed to start event stream: %w", err)
	}
	defer cleanup()

	m := newRemoteRootModel(parsed.BaseURL, service)

	p := tea.NewProgram(m)
	go func() {
		for msg := range stream {
			p.Send(msg)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}
	return nil
}

func newRemoteRootModel(baseURL string, service core.DownloadService) tui.RootModel {
	serverHost, serverPort := parseRemoteServerAddress(baseURL)
	m := tui.InitialRootModel(serverPort, Version, service, nil, false, Commit)
	m.ServerHost = serverHost
	m.ServerPort = serverPort
	m.IsRemote = true
	return m
}

func resolveTokenForConnectTarget(target connectTarget) (string, error) {
	token := strings.TrimSpace(globalToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SURGE_TOKEN"))
	}
	if token != "" {
		return token, nil
	}

	serverHost, _ := parseRemoteServerAddress(target.BaseURL)
	if isLocalHost(serverHost) {
		return ensureAuthToken(), nil
	}
	return "", fmt.Errorf("remote target %q requires authentication: use --token or set SURGE_TOKEN", target.BaseURL)
}

func parseConnectTarget(target string, allowInsecureHTTP bool) (connectTarget, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return connectTarget{}, fmt.Errorf("invalid target: empty target")
	}

	var (
		scheme string
		host   string
		port   string
	)

	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return connectTarget{}, fmt.Errorf("invalid target %q: %w", target, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return connectTarget{}, fmt.Errorf("unsupported scheme %q (use http or https)", u.Scheme)
		}
		if u.Host == "" {
			return connectTarget{}, fmt.Errorf("invalid target %q: missing host", target)
		}
		if u.User != nil {
			return connectTarget{}, fmt.Errorf("invalid target %q: user info is not supported", target)
		}

		scheme = u.Scheme
		host = u.Hostname()
		port = u.Port()
	} else {
		var err error
		host, port, err = net.SplitHostPort(target)
		if err != nil {
			return connectTarget{}, formatConnectTargetAddrError(target, err)
		}
	}

	if host == "" {
		return connectTarget{}, fmt.Errorf("invalid target %q: missing host", target)
	}

	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return connectTarget{}, fmt.Errorf("invalid target %q: invalid port %q", target, port)
		}
	}

	if scheme == "" {
		scheme = "https"
		if isLoopbackHost(host) || isPrivateIPHost(host) {
			scheme = "http"
		}
	}

	if scheme == "http" && !allowInsecureHTTP && !isLoopbackHost(host) && !isPrivateIPHost(host) {
		return connectTarget{}, fmt.Errorf("refusing insecure HTTP for non-loopback target. Use https:// or --insecure-http")
	}

	return connectTarget{
		BaseURL: fmt.Sprintf("%s://%s", scheme, formatConnectURLHost(host, port)),
	}, nil
}

func parseRemoteServerAddress(baseURL string) (string, int) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", 0
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = defaultPortForScheme(u.Scheme)
	}

	return u.Hostname(), port
}

func defaultPortForScheme(scheme string) int {
	switch scheme {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}

func formatConnectTargetAddrError(target string, err error) error {
	msg := err.Error()
	if strings.Contains(msg, "too many colons") {
		return fmt.Errorf("invalid target %q: IPv6 addresses with ports must use brackets, for example [2001:db8::1]:1700", target)
	}
	if strings.Contains(msg, "missing port") {
		return fmt.Errorf("invalid target %q: expected host:port or http(s) URL", target)
	}
	return fmt.Errorf("invalid target %q: %w", target, err)
}

func formatConnectURLHost(host, port string) string {
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(host)
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func isPrivateIPHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsPrivate()
}

func isLocalHost(host string) bool {
	if isLoopbackHost(host) {
		return true
	}
	target := net.ParseIP(host)
	if target == nil {
		return false
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.Equal(target) {
					return true
				}
			case *net.IPAddr:
				if v.IP.Equal(target) {
					return true
				}
			}
		}
	}
	return false
}
