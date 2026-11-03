package main

import (
	"os"
	"strings"
	"testing"
)

// The Linux entrypoint cannot run on every developer host; real UDP behavior
// is covered in collector and full service persistence is a Linux/CGO gate.
func TestProductionUnrestrictedStreamAndRetentionWiring(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, required := range []string{"AcceptRetentionChange: cfg.Accept48hRetention", "RawDays: 2", "RollupDays: 2"} {
		if !strings.Contains(src, required) {
			t.Errorf("missing retention boundary: %s", required)
		}
	}
	for _, forbidden := range []string{"SetAllowedStream(", "EnforceSingleStream: true", "StreamAgent:", "StreamSubAgent:", "VYOS_SFLOW_AGENT"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("obsolete identity gate: %s", forbidden)
		}
	}
}
