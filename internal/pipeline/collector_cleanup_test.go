package pipeline

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/collector"
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func TestCollectorClearSerializesConcurrentIngest(t *testing.T) {
	st, err := store.Open(store.Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := New(Config{}, st)
	c := collector.New(p)
	now := time.Now().UTC().Truncate(time.Second)
	agent := netip.MustParseAddr("192.0.2.1")
	datagram := func(seq uint32, src string) []byte {
		h, size := encode.Frame(netip.MustParseAddr(src), netip.MustParseAddr("198.51.100.1"), 1234, 443, 6, 0, 100, 0x10)
		return encode.SFlow(agent, 7, seq, seq*1000, []encode.SFlowSample{{Seq: seq, SourceID: 1, Rate: 100, Pool: seq * 100, Header: h, FrameLen: size}}, nil)
	}
	for i := uint32(1); i <= 11; i++ {
		if !c.Handle(datagram(i, "10.0.0.1"), agent, now.Add(-time.Minute)) {
			t.Fatal("fixture rejected")
		}
	}
	// Cover a detached run-loop batch as well as the remaining channel queue.
	popped := <-p.in
	p.ingestQueued(<-p.in)
	p.FlushRows()
	p.FlushRollups()
	entered, release := make(chan struct{}), make(chan struct{})
	cleared := make(chan error, 1)
	go func() {
		cleared <- c.ClearHistory(func() error {
			close(entered)
			<-release
			_, err := p.Purge(time.Time{})
			return err
		})
	}()
	<-entered
	// This datagram arrives during clear, after its linearization boundary.
	// It must wait for the collector lock, then survive as new history.
	started, submitted := make(chan struct{}), make(chan bool, 1)
	go func() { close(started); submitted <- c.Handle(datagram(13, "10.0.0.2"), agent, now) }()
	<-started
	close(release)
	select {
	case err := <-cleared:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clear deadlocked with ingest")
	}
	select {
	case ok := <-submitted:
		if !ok {
			t.Fatal("new datagram rejected")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ingest deadlocked with clear")
	}
	p.ingestQueued(popped)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Run(ctx)
	var old, n int
	var packets float64
	if err := st.DB.QueryRow("SELECT count(*) FILTER (WHERE ts < ?),count(*),sum(pkts) FROM hot", now).Scan(&old, &n, &packets); err != nil {
		t.Fatal(err)
	}
	if old != 0 || n != 1 || packets != 200 {
		t.Fatalf("concurrent clear lost or resurrected traffic: old=%d rows=%d packets=%g", old, n, packets)
	}
	if p.Dropped.Load() != 0 || p.WriteErrs.Load() != 0 {
		t.Fatalf("pipeline dropped=%d errors=%d", p.Dropped.Load(), p.WriteErrs.Load())
	}
	var rollPackets float64
	if err := st.DB.QueryRow("SELECT sum(pkts) FROM r_ts").Scan(&rollPackets); err != nil {
		t.Fatal(err)
	}
	if rollPackets != 200 {
		t.Fatalf("rollup packets=%g want 200", rollPackets)
	}
}
