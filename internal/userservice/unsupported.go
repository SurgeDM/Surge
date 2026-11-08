//go:build !linux && !darwin && !windows && !android

package userservice

import "errors"

func New(string) (Manager, error) {
	return nil, errors.New("user services are not supported on this platform")
}
