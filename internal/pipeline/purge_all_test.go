package pipeline

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func TestPurgeAllClearsQueuedBufferedAndPersistedHistory(t *testing.T) {
	opt := store.Options{Dir: t.TempDir(), Threads: 1, EnforceSingleStream: true, StreamAgent: "192.0.2.1", StreamSubAgent: 7}
	st, err := store.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	p := New(Config{}, st)
	now := time.Now().UTC().Truncate(time.Minute)
	rec := func(at time.Time) flow.Record {
		return flow.Record{Start: at, End: at, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("192.0.2.2"), Proto: 6, SrcPort: 50000, DstPort: 443, Bytes: 100, Packets: 1, Mult: 10, SamplingKnown: true, Source: flow.SrcSFlow, Exporter: netip.MustParseAddr("192.0.2.1")}
	}
	p.Ingest([]flow.Record{rec(now.Add(-3 * time.Hour))})
	p.FlushRows()
	p.FlushRollups()
	if err = st.Seal(now); err != nil {
		t.Fatal(err)
	}
	var sealed string
	if err = st.DB.QueryRow("SELECT path FROM segments").Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	p.Ingest([]flow.Record{rec(now.Add(-time.Hour))})
	p.Submit([]flow.Record{rec(now.Add(-time.Minute))})
	// Simulate the run loop having removed a batch from the channel just
	// before cleanup; generation checks must discard it after cleanup too.
	popped := <-p.in
	p.Submit([]flow.Record{rec(now.Add(-2 * time.Minute))})
	if _, err = p.Purge(time.Time{}); err != nil {
		t.Fatal(err)
	}
	p.ingestQueued(popped)
	p.FlushRows()
	p.FlushRollups()
	for _, table := range []string{"hot", "segments", "r_ts", "r_host", "r_dim"} {
		var n int
		if err = st.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("resurrected %s: %d %v", table, n, err)
		}
	}
	if len(p.in) != 0 {
		t.Fatal("queued history not cleared")
	}
	if _, err = os.Stat(sealed); !os.IsNotExist(err) {
		t.Fatalf("sealed file survives full cleanup: %v", err)
	}
	// Post-cleanup traffic is accepted, including a sample in the same minute.
	p.Submit([]flow.Record{rec(now)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Run(ctx)
	var n int
	if err = st.DB.QueryRow("SELECT count(*) FROM hot").Scan(&n); err != nil || n != 1 {
		t.Fatalf("new flow lost: %d %v", n, err)
	}
	st.Close()
	st, err = store.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.DB.QueryRow("SELECT count(*) FROM hot").Scan(&n); err != nil || n != 1 {
		t.Fatalf("restart resurrected history: %d %v", n, err)
	}
	st.Close()
	opt.StreamSubAgent = 8
	if st, err = store.Open(opt); err == nil {
		st.Close()
		t.Fatal("purge removed identity binding")
	}
}
