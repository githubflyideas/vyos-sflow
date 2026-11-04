package pipeline

import (
	"context"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
	"net/netip"
	"testing"
	"time"
)

func TestFailedPurgePreservesPendingHistory(t *testing.T) {
	st, err := store.Open(store.Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := New(Config{}, st)
	at := time.Now().UTC().Truncate(time.Second)
	r := flow.Record{Start: at, End: at, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("192.0.2.2"), Proto: 6, SrcPort: 1234, DstPort: 443, Bytes: 100, Packets: 1, Mult: 100, SamplingKnown: true, Source: flow.SrcSFlow, Exporter: netip.MustParseAddr("192.0.2.1")}
	p.Ingest([]flow.Record{r})
	p.Submit([]flow.Record{r})
	popped := <-p.in
	p.Submit([]flow.Record{r})
	epoch := p.epoch.Load()
	// A real catalog error before any deletion, not a fake storage implementation.
	if _, err := st.DB.Exec("ALTER TABLE segments RENAME TO unavailable_segments"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Purge(time.Time{}); err == nil {
		t.Fatal("expected catalog failure")
	}
	if _, err := st.DB.Exec("ALTER TABLE unavailable_segments RENAME TO segments"); err != nil {
		t.Fatal(err)
	}
	if p.epoch.Load() != epoch || len(p.batch) != 1 || len(p.in) != 1 || len(p.rTS) == 0 {
		t.Errorf("failed purge discarded state: epoch=%d batch=%d queued=%d rollups=%d", p.epoch.Load(), len(p.batch), len(p.in), len(p.rTS))
	}
	p.ingestQueued(popped)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Run(ctx)
	var n int
	var packets float64
	if err := st.DB.QueryRow("SELECT count(*),coalesce(sum(pkts),0) FROM hot").Scan(&n, &packets); err != nil {
		t.Fatal(err)
	}
	if n != 3 || packets != 300 {
		t.Fatalf("failed clear lost accepted traffic: rows=%d packets=%g want 3,300", n, packets)
	}
}
