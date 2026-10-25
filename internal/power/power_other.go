//go:build !windows && !linux && !darwin

package power

import (
	"context"
	"errors"
)

type osController struct{}

func NewController() Controller {
	return osController{}
}

func (osController) Shutdown(context.Context) error {
	return errors.New("shutdown is not supported on this platform")
}

func (osController) AcquireInhibitor(string) (ReleaseFunc, error) {
	return func() error { return nil }, nil
}
