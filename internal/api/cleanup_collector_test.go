package api

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/collector"
	"github.com/githubflyideas/vyos-sflow/internal/encode"
	"github.com/githubflyideas/vyos-sflow/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow/internal/store"
)

// Exercise the real collector -> queued pipeline -> DuckDB -> cleanup handler
// boundary, not a downstream-only clear of a recording sink.
func TestCleanupClearsLiveCollectorReservoir(t *testing.T) {
	st, err := store.Open(store.Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := pipeline.New(pipeline.Config{}, st)
	c := collector.New(p)
	s := &Server{Store: st, Pipe: p, Col: c}
	now := time.Now().UTC().Truncate(time.Second)
	agent := netip.MustParseAddr("192.0.2.1")
	send := func(seq uint32, src string, at time.Time) {
		t.Helper()
		h, size := encode.Frame(netip.MustParseAddr(src), netip.MustParseAddr("198.51.100.1"), 1234, 443, 6, 0, 100, 0x10)
		b := encode.SFlow(agent, 7, seq, seq*1000, []encode.SFlowSample{{Seq: seq, SourceID: 1, Rate: 100, Pool: seq * 100, Header: h, FrameLen: size}}, nil)
		if !c.Handle(b, agent, at) {
			t.Fatal("valid datagram rejected")
		}
	}
	for i := uint32(1); i <= 11; i++ {
		send(i, "10.0.0.1", now.Add(-time.Minute))
	}
	req := httptest.NewRequest("POST", "/api/cleanup", strings.NewReader(`{"all":true,"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.postCleanup(w, req)
	if w.Code != 200 {
		t.Fatalf("cleanup: %d %s", w.Code, w.Body)
	}
	send(13, "10.0.0.2", now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Run(ctx)
	var old, n int
	var packets float64
	if err := st.DB.QueryRow("SELECT count(*) FILTER (WHERE ts < ?), count(*), sum(pkts) FROM hot", now).Scan(&old, &n, &packets); err != nil {
		t.Fatal(err)
	}
	if old != 0 || n != 1 || packets != 100 {
		t.Fatalf("post-clear history: old=%d rows=%d packets=%g; want 0,1,100", old, n, packets)
	}
	if p.WriteErrs.Load() != 0 {
		t.Fatal(p.LastError())
	}
	sources := c.Sources()
	if len(sources) != 1 || sources[0].Sampling != "1:100" || sources[0].Records != 12 {
		t.Fatalf("cleanup changed source diagnostics: %+v", sources)
	}
}
