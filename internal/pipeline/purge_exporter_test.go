package pipeline

import (
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"github.com/githubflyideas/vyos-sflow/internal/store"
)

// Deleting one device takes its traffic out of every view: recent rows,
// sealed hourly files, and long ranges that the summaries would answer.
func TestPurgeExporter(t *testing.T) {
	p, st := setup(t)
	now := time.Now().UTC().Truncate(time.Hour)
	good, bad := netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.66")
	var recs []flow.Record
	for i := 0; i < 600; i++ { // ten hours, one record a minute from each
		ts := now.Add(-10*time.Hour + time.Duration(i)*time.Minute)
		for i, e := range []netip.Addr{good, bad} {
			cli := []netip.Addr{netip.MustParseAddr("10.1.1.1"), netip.MustParseAddr("10.1.1.2")}[i] // different traffic, not one flow seen twice
			recs = append(recs, flow.Record{Start: ts, End: ts.Add(30 * time.Second), Src: cli, Dst: netip.MustParseAddr("198.51.100.9"),
				SrcPort: 40000, DstPort: 443, Proto: 6, Bytes: 1000, Packets: 1, Mult: 1, SamplingKnown: true, L2: true, Exporter: e, Source: flow.SrcSFlow,
				InIf: 1})
		}
	}
	p.Ingest(recs)
	p.FlushRows()
	p.FlushRollups()
	if err := st.Seal(now.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	long := store.Query{From: now.Add(-12 * time.Hour), To: now} // whole hours, over 6 h: summaries
	before, err := st.Totals(long)
	if err != nil || before.Wire != 2*600*1000 {
		t.Fatalf("before: %+v %v", before, err)
	}
	done, err := p.PurgeExporter(bad.String())
	if err != nil || done.Rows != 600 {
		t.Fatalf("purge: %+v %v", done, err)
	}
	for _, q := range []store.Query{long, {From: now.Add(-2 * time.Hour), To: now}} {
		got, err := st.Totals(q)
		want := uint64(q.To.Sub(q.From) / time.Minute * 1000)
		if want > 600*1000 {
			want = 600 * 1000
		}
		if err != nil || got.Wire != want {
			t.Fatalf("after, %v: wire %d %v, want %d", q.To.Sub(q.From), got.Wire, err, want)
		}
	}
	exps, _ := st.StoredExporters()
	if len(exps) != 1 || exps[0].Exporter != good.String() {
		t.Fatalf("stored exporters %+v", exps)
	}
}

// Flows both devices reported, the deleted one first, in step: the other
// device's copies were duplicates and must count once it is gone.
func TestPurgeExporterKeepsSharedFlows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	mk := func(e string, ts time.Time) flow.Record {
		return flow.Record{Start: ts, End: ts.Add(30 * time.Second), Src: netip.MustParseAddr("10.1.1.1"), Dst: netip.MustParseAddr("198.51.100.9"),
			SrcPort: 40000, DstPort: 443, Proto: 6, Bytes: 1000, Packets: 1, Mult: 1, SamplingKnown: true, L2: true,
			Exporter: netip.MustParseAddr(e), Source: flow.SrcSFlow, InIf: 1}
	}
	p, st := setup(t)
	p2, st2 := setup(t)
	for i := 0; i < 600; i++ {
		ts := now.Add(-10*time.Hour + time.Duration(i)*time.Minute)
		p.Ingest([]flow.Record{mk("10.0.0.66", ts), mk("10.0.0.1", ts)})
		p2.Ingest([]flow.Record{mk("10.0.0.1", ts)})
	}
	for _, x := range []*Pipeline{p, p2} {
		x.FlushRows()
		x.FlushRollups()
	}
	for _, s := range []*store.Store{st, st2} {
		if err := s.Seal(now.Add(-3 * time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if p.DupRows.Load() == 0 {
		t.Fatal("no duplicates to begin with")
	}
	if _, err := p.PurgeExporter("10.0.0.66"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []store.Query{{From: now.Add(-12 * time.Hour), To: now}, {From: now.Add(-2 * time.Hour), To: now}} {
		a, _ := st.Totals(q)
		b, _ := st2.Totals(q)
		if a.Wire != b.Wire || a.Wire == 0 {
			t.Fatalf("%v: after purge wire %d, the other device alone %d", q.To.Sub(q.From), a.Wire, b.Wire)
		}
	}
}
