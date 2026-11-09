package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/tui"
)

// backend returns only after its producers, pipeline and storage have stopped.
// A ready message carries the address of the already-bound HTTP listener.
type sessionHooks struct {
	backend      func(context.Context, string, string, chan<- tui.Options) error
	view         func(context.Context, tui.Options) error
	tempRoot     string
	readyTimeout time.Duration
}

func runOwnedTUI(ctx context.Context, listen, lang string, hooks sessionHooks) (result error) {
	dir, err := os.MkdirTemp(hooks.tempRoot, "vyos-sflow-tui-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(dir)) }()
	ctx, cancel := context.WithCancel(ctx)
	ready := make(chan tui.Options, 1)
	done := make(chan error, 1)
	go func() { done <- hooks.backend(ctx, dir, listen, ready) }()
	joined := false
	defer func() {
		cancel()
		if !joined {
			result = errors.Join(result, <-done)
		}
	}()
	timeout := hooks.readyTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var opt tui.Options
	select {
	case <-timer.C:
		return errors.New("temporary backend readiness timed out")
	case opt = <-ready:
	case err := <-done:
		joined = true
		if err == nil && ctx.Err() == nil {
			return errors.New("temporary backend exited before readiness")
		}
		return err
	}
	opt.Lang = lang
	viewDone := make(chan error, 1)
	go func() { viewDone <- hooks.view(ctx, opt) }()
	select {
	case err := <-viewDone:
		return err
	case err := <-done:
		joined = true
		cancel()
		return errors.Join(err, <-viewDone)
	case <-ctx.Done():
		return <-viewDone
	}
}
