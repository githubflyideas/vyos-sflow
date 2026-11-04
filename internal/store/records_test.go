package store

import (
	"testing"
	"time"
)

// The pages ask for clients and servers apart, for what went in or out
// through the interface sFlow samples on, and for the newest records.
func TestRecordsDirectionsAndEnds(t *testing.T) {
	s, err := Open(Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	base := Row{TS: ts, Client: "192.0.2.10", Server: "198.51.100.7", Proto: 6, SvcPort: 443, App: "HTTPS", Pkts: 1, Flows: 1, Exporter: "10.0.2.15"}
	down, up, other := base, base, base
	down.Src, down.Dst, down.Wire, down.InIf, down.OutIf = base.Server, base.Client, 9000, 2, 3 // in through eth0 (2)
	up.Src, up.Dst, up.Wire, up.InIf, up.OutIf, up.TS = base.Client, base.Server, 1000, 3, 2, ts.Add(time.Minute)
	other.Client, other.Src, other.Dst, other.Wire, other.InIf, other.OutIf = "192.0.2.11", "192.0.2.11", base.Server, 500, 4, 5
	// traffic to the router itself names the local interface on the other side
	local := down
	local.Wire, local.InIf, local.OutIf = 50000, 2, LocalIf
	if err := s.AppendRows([]Row{down, up, other, local}); err != nil {
		t.Fatal(err)
	}
	q := Query{From: ts.Add(-time.Hour), To: ts.Add(time.Hour)}
	if k, err := s.BusiestIface(q); err != nil || k != "10.0.2.15/2" {
		t.Fatalf("busiest interface %q %v", k, err)
	}
	for f, want := range map[string]uint64{"iface_in": 59000, "iface_out": 1000, "iface": 60000} {
		qq := q
		qq.Filters = []Filter{{Field: f, Value: "10.0.2.15/2"}}
		tot, err := s.Totals(qq)
		if err != nil || tot.Wire != want {
			t.Errorf("%s: %d %v, want %d", f, tot.Wire, err, want)
		}
	}
	cl, err := s.TopN(q, "client", 10)
	if err != nil || len(cl) != 2 || cl[0].Key != "192.0.2.10" || cl[0].Wire != 60000 {
		t.Fatalf("clients %+v %v", cl, err)
	}
	sv, err := s.TopN(q, "server", 10)
	if err != nil || len(sv) != 1 || sv[0].Wire != 60500 {
		t.Fatalf("servers %+v %v", sv, err)
	}
	rs, err := s.Records(q, 2)
	if err != nil || len(rs) != 2 || rs[0].Wire != 1000 || rs[0].Client != "192.0.2.10" || rs[0].SvcPort != 443 {
		t.Fatalf("records %+v %v", rs, err)
	}
}

// On a router that only talks for itself (the downloads are its own),
// every packet names its port on one side and the local interface on the
// other, the two equally busy: the port is the one meant.
func TestBusiestIfaceNotLocal(t *testing.T) {
	s, err := Open(Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	base := Row{TS: ts, Client: "10.0.2.15", Server: "203.0.113.5", Proto: 6, SvcPort: 443, App: "HTTPS", Pkts: 1, Flows: 1, Exporter: "10.0.2.15"}
	in, out := base, base
	in.Src, in.Dst, in.Wire, in.InIf, in.OutIf = base.Server, base.Client, 9000, 2, LocalIf
	out.Src, out.Dst, out.Wire, out.InIf, out.OutIf = base.Client, base.Server, 1000, LocalIf, 2
	if err := s.AppendRows([]Row{in, out}); err != nil {
		t.Fatal(err)
	}
	if k, err := s.BusiestIface(Query{From: ts.Add(-time.Hour), To: ts.Add(time.Hour)}); err != nil || k != "10.0.2.15/2" {
		t.Fatalf("busiest interface %q %v, want the port", k, err)
	}
}
