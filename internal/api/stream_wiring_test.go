package api

import (
	"os"
	"strings"
	"testing"
)

func TestStreamDiagnosticAndQueryWiring(t *testing.T) {
	for file, required := range map[string][]string{
		"cleanup.go": {`validateCleanupRequest(r)`, `s.Pipe.Purge(time.Time{})`, `"estimate": all`},

		"handlers.go": {`"stream": s.Col.StreamStatus()`, `"history_days": 2`},
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range required {
			if !strings.Contains(string(b), text) {
				t.Errorf("%s missing %s", file, text)
			}
		}
	}
}
