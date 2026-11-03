package main

import (
	"os"
	"strings"
	"testing"
)

// Wiring evidence is runnable without DuckDB/CGO; Linux lifecycle tests below
// and the PTY acceptance exercise the actual backend, not this source check.
func TestLinuxEntryPointsShareContextOwnedServe(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	for _, required := range []string{"func serveConfigured(", "return serveConfigured(ctx, cfg, os.Getenv(\"VYOS_SFLOW_PASSWORD\"), nil)", "return serveConfigured(ctx, cfg, auth.Generate(), ready)", "return runTUICommand(ctx, args, commandHooks{", "syscall.SIGHUP"} {
		if !strings.Contains(source, required) {
			t.Errorf("missing lifecycle wiring: %s", required)
		}
	}
}
