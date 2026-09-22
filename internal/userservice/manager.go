package userservice

import "context"

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
