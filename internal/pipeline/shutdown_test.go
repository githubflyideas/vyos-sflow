package pipeline

import (
	"context"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
	"net/netip"
	"testing"
	"time"
)

func TestCancelDrainsQueuedRecords(t *testing.T) {
	p, st := setup(t)
	now := time.Now().UTC()
	r := flow.Record{Start: now, End: now, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("192.0.2.1"), Exporter: netip.MustParseAddr("192.0.2.2"), Bytes: 100, Packets: 1, Mult: 1, L2: true, Source: flow.SrcSFlow}
	for i := 0; i < 100; i++ {
		p.Submit([]flow.Record{r})
	}
	p.SubmitCounters([]flow.IfCounters{{Time: now, Exporter: r.Exporter, IfIndex: 1, InOctets: 123}})
	if p.CounterSamples.Load() != 1 {
		t.Fatal("counter receipt not counted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go p.Run(ctx)
	select {
	case <-p.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	p.Submit([]flow.Record{r})
	p.SubmitCounters([]flow.IfCounters{{}})
	if p.Dropped.Load() != 1 || p.DroppedCounters.Load() != 1 {
		t.Fatal("post-shutdown submissions not counted")
	}
	if got := p.Records.Load(); got != 100 {
		t.Fatalf("processed %d queued records, want 100", got)
	}
	if p.WriteErrs.Load() != 0 {
		t.Fatalf("shutdown writes failed: %s", p.LastError())
	}
	if len(p.in) != 0 {
		t.Fatal("cancellation left accepted data queued")
	}
	got, err := st.Totals(store.Query{From: now.Add(-time.Minute), To: now.Add(time.Minute)})
	if err != nil || got.Wire != 10000 {
		t.Fatalf("drained rows not flushed: %+v %v", got, err)
	}
}
