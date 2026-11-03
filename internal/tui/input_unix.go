//go:build linux || darwin

package tui

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Polling avoids a stranded stdin read on q or a signal. It does not close
// stdin, change shared descriptor flags, or leave a helper process behind.
type terminalInput struct {
	ctx context.Context
	fd  int
}

func (r terminalInput) Read(b []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		fds := []unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&unix.POLLIN != 0 {
			n, err := unix.Read(r.fd, b)
			if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
				continue
			}
			if n == 0 && err == nil {
				return 0, io.EOF
			}
			return n, err
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
	}
}
func startKeys(ctx context.Context, out chan<- string) func() {
	done := make(chan struct{})
	go func() { defer close(done); readKeysFrom(ctx, terminalInput{ctx: ctx, fd: int(os.Stdin.Fd())}, out) }()
	return func() { <-done }
}
