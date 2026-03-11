package runtime

import (
	"context"
	"errors"
)

var ErrServiceUnavailable = errors.New("runtime service unavailable")

func (a *App) Subscribe(ctx context.Context) (<-chan interface{}, func(), error) {
	if a == nil || a.service == nil {
		return nil, nil, ErrServiceUnavailable
	}
	return a.service.StreamEvents(ctx)
}

func (a *App) Publish(msg interface{}) error {
	if a == nil || a.service == nil {
		return ErrServiceUnavailable
	}
	return a.service.Publish(msg)
}
