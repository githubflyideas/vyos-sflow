//go:build linux || darwin

package tui

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPollingInputStopsOnCancellationAndEOF(t *testing.T) {
	for _, ending := range []string{"cancel", "EOF"} {
		t.Run(ending, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { var b [1]byte; _, err := (terminalInput{ctx: ctx, fd: int(r.Fd())}).Read(b[:]); done <- err }()
			if ending == "cancel" {
				cancel()
			} else {
				w.Close()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("missing read termination")
				}
			case <-time.After(time.Second):
				t.Fatal("input goroutine leaked")
			}
		})
	}
}
