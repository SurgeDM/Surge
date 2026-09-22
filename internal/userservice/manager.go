package userservice

import (
	"context"
	"errors"
)

var ErrRootInstall = errors.New("Surge user services cannot be managed from an elevated session; run this command again as your normal user")
var ErrLegacySystemService = errors.New("a legacy system-wide Surge service is installed")

type State int

const (
	NotInstalled State = iota
	Stopped
	Running
)

type Manager interface {
	Install(context.Context) error
	Uninstall(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
	Restart(context.Context) error
	Status(context.Context) (State, error)
}
