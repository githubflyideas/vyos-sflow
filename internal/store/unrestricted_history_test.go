package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnrestrictedOpenPreservesHistoricalBindingAndData(t *testing.T) {
	now := time.Now().UTC()
	opt := Options{Dir: t.TempDir(), Threads: 1, Now: func() time.Time { return now }, EnforceSingleStream: true, StreamAgent: "192.0.2.1", StreamSubAgent: 100000}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			s.Close()
		}
	})
	if err = s.AppendRows([]Row{flowRow(now.Add(-time.Hour), 123)}); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(opt.Dir, "existing-account-sentinel")
	if err = os.WriteFile(sentinel, []byte("do not overwrite"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	opt.EnforceSingleStream = false
	opt.StreamAgent = ""
	opt.StreamSubAgent = 0
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{streamAgentKey: "192.0.2.1", streamSubAgentKey: "100000"} {
		var got string
		if err = s.DB.QueryRow("SELECT v FROM meta WHERE k=?", k).Scan(&got); err != nil || got != want {
			t.Fatalf("metadata %s=%q err=%v", k, got, err)
		}
	}
	var count int
	var wire uint64
	if err = s.DB.QueryRow("SELECT count(*),sum(wire) FROM hot").Scan(&count, &wire); err != nil || count != 1 || wire != 123 {
		t.Fatalf("history changed: %d/%d %v", count, wire, err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "do not overwrite" {
		t.Fatalf("existing file changed: %q %v", b, err)
	}
}
