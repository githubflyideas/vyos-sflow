package pipeline

import (
	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"net/netip"
	"testing"
	"time"
)

func TestSFlowMetadataAndSamplingSurviveProductCut(t *testing.T) {
	p, st := setup(t)
	now := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	r := flow.Record{Start: now, End: now, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("192.0.2.1"), SrcPort: 45000, DstPort: 443, Proto: 6, Bytes: 128, Packets: 1, L2: true, Mult: 1000, SamplingKnown: true, Exporter: netip.MustParseAddr("10.0.0.254"), InIf: 0x12345678, OutIf: 7, Direction: 1, Observation: 0x12345678, Source: flow.SrcSFlow}
	p.Ingest([]flow.Record{r})
	p.FlushRows()
	p.FlushRollups()
	var in, out uint32
	var mult float32
	var wire uint64
	var known bool
	if err := st.DB.QueryRow("SELECT in_if,out_if,mult,wire,samp_known FROM hot").Scan(&in, &out, &mult, &wire, &known); err != nil {
		t.Fatal(err)
	}
	if in != r.InIf || out != r.OutIf || mult != 1000 || wire != 128000 || !known {
		t.Fatalf("metadata/scaling changed: %d %d %f %d %v", in, out, mult, wire, known)
	}
	var enriched int
	if err := st.DB.QueryRow("SELECT count(*) FROM hot WHERE peer_cc<>'' OR peer_asn<>0 OR peer_org<>'' OR segment<>'' OR threat<>''").Scan(&enriched); err != nil || enriched != 0 {
		t.Fatalf("unexpected enrichment: %d %v", enriched, err)
	}
	var advanced int
	if err := st.DB.QueryRow("SELECT count(*) FROM r_dim WHERE dim NOT IN ('app','proto')").Scan(&advanced); err != nil || advanced != 0 {
		t.Fatalf("advanced rollups: %d %v", advanced, err)
	}
}

func TestDedupKeepsExpandedObservationAndDirection(t *testing.T) {
	p := New(Config{}, nil)
	now := time.Now().UTC()
	key := dkey{src: netip.MustParseAddr("10.0.0.1"), dst: netip.MustParseAddr("192.0.2.1")}
	a := origin{exporter: netip.MustParseAddr("10.0.0.254"), obs: 0x12345678, dir: 1}
	if p.isDup(now, key, a) || p.isDup(now, key, a) {
		t.Fatal("same observation must not be a duplicate")
	}
	b := a
	b.obs = 0x345678
	if !p.isDup(now, key, b) {
		t.Fatal("expanded observation was truncated")
	}
	b = a
	b.dir = 0
	if !p.isDup(now, key, b) {
		t.Fatal("direction disappeared from observation identity")
	}
}
