package cmd

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/tui"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

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
			cmd.PrintErrf("Auto-detected local server on port %d\n", port)
		}
		return connectAndRunTUI(cmd, target)
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func connectAndRunTUI(cmd *cobra.Command, target string) error {
	clientCfg := currentRemoteClientConfig()
	parsed, err := parseConnectTarget(target, clientCfg.AllowInsecureHTTP)
	if err != nil {
		return err
	}

	token, err := resolveTokenForConnectTarget(parsed)
	if err != nil {
		return err
	}

	cmd.PrintErrf("Connecting to %s...\n", parsed.BaseURL)

	service, err := newRemoteDownloadService(parsed.BaseURL, token)
	if err != nil {
		return fmt.Errorf("failed to configure remote client: %w", err)
	}
	defer func() { _ = service.Shutdown() }()

	streamCtx, cancelStream := context.WithCancel(cmd.Context())
	stream, cleanup, err := service.StreamEvents(streamCtx)
	if err != nil {
		cancelStream()
		return fmt.Errorf("failed to start event stream: %w", err)
	}

	connectCtx, cancelConnect := context.WithTimeout(cmd.Context(), clientCfg.ConnectTimeout)
	statuses, err := service.ListContext(connectCtx)
	cancelConnect()
	if err != nil {
		cancelStream()
		cleanup()
		for range stream {
		}
		return fmt.Errorf("failed to connect: %w", err)
	}

	tui.InitializeTUI()
	settings := globalSettings
	if settings == nil {
		settings = getSettings()
	}
	m := newRemoteRootModel(parsed.BaseURL, service, statuses, settings)

	p := tea.NewProgram(m)
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		for msg := range stream {
			p.Send(msg)
		}
	}()

	_, runErr := p.Run()
	cancelStream()
	cleanup()
	<-forwardDone
	if runErr != nil {
		return fmt.Errorf("error running TUI: %w", runErr)
	}
	return nil
}

func newRemoteRootModel(baseURL string, service service.DownloadService, statuses []types.DownloadStatus, settings *config.Settings) tui.RootModel {
	serverHost, serverPort := parseRemoteServerAddress(baseURL)
	m := tui.InitialRootModelWithStatuses(serverPort, Version, service, nil, settings, false, statuses, Commit)
	m.ServerHost = serverHost
	m.ServerPort = serverPort
	m.IsRemote = true
	return m
}
