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

var connectCmd = &cobra.Command{
	Use:   "connect [host:port]",
	Short: "Connect TUI to a running Surge daemon",
	Long:  `Connect to a running Surge daemon and open the TUI. When no target is specified, auto-detects a locally running server.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var target string
		if len(args) > 0 {
			target = args[0]
		} else {
			port := readActivePort()
			if port == 0 {
				return fmt.Errorf("no local Surge server detected. Start one with 'surge' or 'surge server', or specify a target: surge connect <host:port>")
			}
			target = fmt.Sprintf("127.0.0.1:%d", port)
			fmt.Printf("Auto-detected local server on port %d\n", port)
		}
		return connectAndRunTUI(cmd, target)
	},
}

func init() {
	connectCmd.Flags().Bool("insecure-http", false, "Allow plain HTTP for non-loopback targets")
	rootCmd.AddCommand(connectCmd)
}

func connectAndRunTUI(cmd *cobra.Command, target string) error {
	insecureHTTP, _ := cmd.Flags().GetBool("insecure-http")
	baseURL, err := resolveConnectBaseURL(target, insecureHTTP)
	if err != nil {
		return err
	}

	token, err := resolveTokenForTarget(target)
	if err != nil {
		return err
	}

	fmt.Printf("Connecting to %s...\n", baseURL)

	service := core.NewRemoteDownloadService(baseURL, token)
	_, err = service.List()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	stream, cleanup, err := service.StreamEvents(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start event stream: %w", err)
	}
	defer cleanup()

	port := 0
	serverHost := hostnameFromTarget(target)
	if u, err := url.Parse(baseURL); err == nil {
		if h := u.Hostname(); h != "" {
			serverHost = h
		}
		if p := u.Port(); p != "" {
			port, _ = strconv.Atoi(p)
		}
	}

	m := newRemoteRootModel(port, service, serverHost)

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

func newRemoteRootModel(port int, service core.DownloadService, serverHost string) tui.RootModel {
	m := tui.InitialRootModel(port, Version, service, nil, false)
	m.ServerHost = serverHost
	m.IsRemote = true
	return m
}

func resolveTokenForTarget(target string) (string, error) {
	token := strings.TrimSpace(globalToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("SURGE_TOKEN"))
	}
	if token != "" {
		return token, nil
	}

	host := target
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err == nil {
			host = u.Hostname()
		}
	} else {
		host = hostnameFromTarget(target)
	}

	if isLocalHost(host) {
		return ensureAuthToken(), nil
	}
	return "", fmt.Errorf("no token provided. Use --token or set SURGE_TOKEN")
}

func resolveConnectBaseURL(target string, allowInsecureHTTP bool) (string, error) {
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return "", fmt.Errorf("invalid target: %v", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("unsupported scheme %q (use http or https)", u.Scheme)
		}
		if u.Host == "" {
			return "", fmt.Errorf("invalid target: missing host")
		}
		host := u.Hostname()
		if u.Scheme == "http" && !allowInsecureHTTP && !isLoopbackHost(host) && !isPrivateIPHost(host) {
			return "", fmt.Errorf("refusing insecure HTTP for non-loopback target. Use https:// or --insecure-http")
		}
		return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
	}

	scheme := "https"
	host := hostnameFromTarget(target)
	if isLoopbackHost(host) || isPrivateIPHost(host) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, target), nil
}

func hostnameFromTarget(target string) string {
	host := target
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
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
