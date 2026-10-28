package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/SurgeDM/Surge/internal/clioutput"
)

type commandSignals struct {
	Context context.Context
	Force   <-chan struct{}
	stop    func()
}

// The first interrupt cancels command work; another releases a bounded shutdown
// wait. No signal handler terminates the process or mutates shared daemon jobs.
func captureCommandSignals(parent context.Context) commandSignals {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	force, done, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var forceOnce, stopOnce sync.Once
	go func() {
		defer close(finished)
		for {
			select {
			case sig := <-signals:
				if ctx.Err() != nil {
					forceOnce.Do(func() { close(force) })
					continue
				}
				code, message := clioutput.Interrupted, "Interrupted; only invocation-owned downloads were affected."
				if sig == syscall.SIGTERM {
					code, message = clioutput.Terminated, "Terminated; only invocation-owned downloads were affected."
				}
				cancel(clioutput.NewError(code, message, nil))
			case <-done:
				return
			}
		}
	}()
	return commandSignals{Context: ctx, Force: force, stop: func() {
		stopOnce.Do(func() { signal.Stop(signals); close(done); cancel(context.Canceled); <-finished })
	}}
}
