//go:build !darwin && !linux && !windows && !android

package userservice

import (
	"context"
	"errors"
)

// ErrUnsupportedPlatform indicates that user services are not supported on this operating system.
var ErrUnsupportedPlatform = errors.New("user service is not supported on this platform")

type unsupportedManager struct{}

// New returns a stub Manager on platforms where user services are not supported.
func New(_ string) (Manager, error) {
	return &unsupportedManager{}, nil
}

func (m *unsupportedManager) Install(context.Context) error {
	return ErrUnsupportedPlatform
}

func (m *unsupportedManager) Uninstall(context.Context) error {
	return ErrUnsupportedPlatform
}

func (m *unsupportedManager) Start(context.Context) error {
	return ErrUnsupportedPlatform
}

func (m *unsupportedManager) Stop(context.Context) error {
	return ErrUnsupportedPlatform
}

func (m *unsupportedManager) Restart(context.Context) error {
	return ErrUnsupportedPlatform
}

func (m *unsupportedManager) Status(context.Context) (State, error) {
	return NotInstalled, ErrUnsupportedPlatform
}
