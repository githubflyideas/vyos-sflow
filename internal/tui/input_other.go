//go:build !linux && !darwin

package tui

import (
	"context"
	"os"
)

// The production command is Linux-only. Preserve the portable renderer's
// existing input behavior on other platforms used for its pure-Go tests.
func startKeys(ctx context.Context, out chan<- string) func() {
	go readKeysFrom(ctx, os.Stdin, out)
	return func() {}
}
