package sim

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"testing"
	"time"
)

func TestBackfillOnlySFlowObservations(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	s := New(now, 7)
	n := 0
	s.Backfill(now.Add(-7*time.Hour), now, true, func(rs []flow.Record, _ []flow.IfCounters) {
		for _, r := range rs {
			n++
			if r.Source != flow.SrcSFlow || !r.L2 || r.Start != r.End || r.Observation != uint64(r.InIf) {
				t.Fatalf("not a normalized synthetic sFlow observation: %+v", r)
			}
		}
	})
	if n == 0 {
		t.Fatal("empty simulation")
	}
}
