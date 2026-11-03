package tui

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBasicPagesOnlyRequestSupportedAPI(t *testing.T) {
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		if r.URL.Query().Get("from") != "" || r.URL.Query().Get("f") != "" {
			t.Errorf("deep query remains: %s", r.URL)
		}
		switch r.URL.Path {
		case "/api/overview":
			w.Write([]byte(`{"totals":{"wire":100,"pkts":1,"flows":1},"now_bps":8}`))
		case "/api/topn":
			if !strings.Contains("|host|app|proto|", "|"+r.URL.Query().Get("dim")+"|") {
				t.Errorf("unsupported dimension: %s", r.URL)
			}
			w.Write([]byte(`{"rows":[{"key":"test","wire":100,"pkts":1,"flows":1}]}`))
		case "/api/series":
			w.Write([]byte(`{"names":["HTTPS"],"values":[[1,2]]}`))
		case "/api/sources":
			w.Write([]byte(`{"stream":{"enforced":true,"configured":true,"agent":"192.0.2.1","sub_agent":7,"rejected_streams":19},"sources":[{"exporter":"192.0.2.1","proto":"sFlow v5","sampling":"1:100","status":"warn","lost_pct":2,"transit_pct":3,"drop_pct":7,"issues":["loss","agent_drops"]}]}`))
		case "/api/status":
			w.Write([]byte(`{"counter_samples":23,"dropped_counters":31,"write_errors":41,"last_error":"disk full"}`))
		default:
			t.Errorf("removed API requested: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a := newApp(Options{URL: srv.URL, Lang: "en"})
	for pg := range pageKeys {
		for rg := range ranges {
			for dm := range dims {
				p := a.load(view{pg: pg, rng: rg, dim: dm, w: 120})
				if p.err != "" {
					t.Fatal(p.err)
				}
				if pg == 3 {
					var text strings.Builder
					for _, l := range p.top {
						text.WriteString(l.plain())
					}
					for _, sec := range p.sections {
						for _, row := range sec.rows {
							for _, l := range row.detail {
								text.WriteString(l.plain())
							}
						}
					}
					for _, want := range []string{"23", "31", "41", "disk full", "3.0%", "7.0%", "sFlow agent 192.0.2.1 / sub-agent 7", "Rejected stream datagrams: 19"} {
						if !strings.Contains(text.String(), want) {
							t.Fatalf("missing diagnostic %s: %s", want, text.String())
						}
					}
				}
			}
		}
	}
	if len(seen) != 5 {
		t.Fatalf("API coverage: %v", seen)
	}
}

func TestHistoryRangesMatch48HourBoundary(t *testing.T) {
	want := []string{"15m", "1h", "6h", "24h", "48h"}
	if !slices.Equal(ranges, want) {
		t.Fatalf("ranges: %v", ranges)
	}
	if rangeDur[len(rangeDur)-1] != 48*time.Hour {
		t.Fatal("48h duration mismatch")
	}
	a := newApp(Options{Lang: "en"})
	if ranges[a.rng] != "1h" {
		t.Fatal("default range is not 1h")
	}
	for i, r := range want {
		if query(i, nil).Get("range") != r {
			t.Fatal("range request mismatch")
		}
	}
}

func TestUnrestrictedStreamNeedsNoIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"stream":{"enforced":false,"configured":false},"sources":[]}`))
	}))
	defer srv.Close()
	a := newApp(Options{URL: srv.URL, Lang: "en"})
	p := a.load(view{pg: 3, rng: 1, w: 120})
	if p.err != "" {
		t.Fatal(p.err)
	}
	for _, l := range p.top {
		if strings.Contains(l.plain(), "agent") || strings.Contains(l.plain(), "stream.") || strings.Contains(l.plain(), "No samples") {
			t.Fatalf("unrestricted receiver demands identity: %s", l.plain())
		}
	}
}

func TestUnconfiguredStreamDiagnostics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"stream":{"enforced":true,"configured":false,"agent":"","sub_agent":0,"rejected_streams":19},"sources":[]}`))
	}))
	defer srv.Close()
	a := newApp(Options{URL: srv.URL, Lang: "en"})
	p := a.load(view{pg: 3, rng: 1, w: 120})
	var text strings.Builder
	for _, l := range p.top {
		text.WriteString(l.plain())
	}
	for _, forbidden := range []string{"No samples are accepted", "-agent", "stream.unconfigured"} {
		if strings.Contains(text.String(), forbidden) {
			t.Fatalf("obsolete identity requirement: %s", text.String())
		}
	}
}
