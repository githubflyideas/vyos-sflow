package tui

import (
	"fmt"
	"math"
)

type streamStatus struct {
	Enforced   bool   `json:"enforced"`
	Configured bool   `json:"configured"`
	Agent      string `json:"agent"`
	SubAgent   uint32 `json:"sub_agent"`
	Rejected   uint64 `json:"rejected_streams"`
}

type topRow struct {
	Key   string  `json:"key"`
	Key2  string  `json:"key2"`
	Key3  string  `json:"key3"`
	Extra string  `json:"extra"`
	Wire  float64 `json:"wire"`
	Pkts  float64 `json:"pkts"`
	Flows float64 `json:"flows"`
	Peers float64 `json:"peers"`
}

var protoNames = map[string]string{"1": "ICMP", "6": "TCP", "17": "UDP", "47": "GRE", "50": "ESP", "58": "ICMPv6", "132": "SCTP"}

func txt(s string) line        { return line{{"", s}} }
func styled(st, s string) line { return line{{st, s}} }
func spark(vals []float64, width int, maxv float64) string {
	if len(vals) == 0 || width <= 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, 0, width)
	for c := 0; c < width; c++ {
		lo := c * len(vals) / width
		hi := max(lo+1, (c+1)*len(vals)/width)
		v, n := 0.0, 0
		for _, x := range vals[lo:min(hi, len(vals))] {
			if x >= 0 {
				v += x
				n++
			}
		}
		if n == 0 {
			out = append(out, ' ')
			continue
		}
		v /= float64(n)
		i := 0
		if maxv > 0 {
			i = int(math.Round(v / maxv * 7))
		}
		out = append(out, blocks[max(0, min(7, i))])
	}
	return string(out)
}

// measure turns bytes and packets over the range into the chosen unit.
type measure struct {
	unit int
	secs float64
}

func (m measure) val(wire, pkts float64) float64 {
	switch units[m.unit] {
	case "bytes":
		return wire
	case "pps":
		return pkts / m.secs
	}
	return wire * 8 / m.secs
}

func (m measure) fmt(v float64) string {
	switch units[m.unit] {
	case "bytes":
		return fmtBytes(v)
	case "pps":
		return fmtPps(v)
	}
	return fmtBps(v)
}

// load fetches only the basic local-analysis pages.
func (a *app) load(v view) page {
	var p page
	fail := func(err error) page { p.err = err.Error(); return p }
	q := query(v.rng, nil)
	m := measure{v.unit, rangeDur[v.rng].Seconds()}
	switch pageKeys[v.pg] {
	case "overview":
		var d struct {
			NowBPS float64 `json:"now_bps"`
			Totals struct{ Wire, Pkts, Flows float64 }
		}
		if err := a.get("overview", q, &d); err != nil {
			return fail(err)
		}
		p.top = []line{txt(a.t("kpi.now") + ": " + fmtBps(d.NowBPS)), txt(a.t("col.traffic") + ": " + fmtBytes(d.Totals.Wire)), txt(fmt.Sprintf("%s: %.0f  %s: %.0f", a.t("col.pkts"), d.Totals.Pkts, a.t("col.flows"), d.Totals.Flows))}
	case "top":
		dm := dims[v.dim]
		q.Set("dim", dm)
		q.Set("limit", "66")
		var d struct{ Rows []topRow }
		if err := a.get("topn", q, &d); err != nil {
			return fail(err)
		}
		label := a.t("col.proto")
		if dm == "host" {
			label = a.t("kpi.hosts")
		}
		if dm == "app" {
			label = a.t("col.service")
		}
		p.top = []line{txt("← " + label + " →")}
		sec := section{cols: []column{{head: label}, {head: a.t("tui.unit." + units[v.unit]), width: 12, right: true}, {head: a.t("col.flows"), width: 10, right: true}}}
		for _, r := range d.Rows {
			name := r.Key
			if dm == "proto" && protoNames[name] != "" {
				name = protoNames[name]
			}
			sec.rows = append(sec.rows, row{key: r.Key, cells: []line{txt(name), txt(m.fmt(m.val(r.Wire, r.Pkts))), txt(fmt.Sprintf("%.0f", r.Flows))}})
		}
		p.sections = []section{sec}
	case "traffic":
		q.Set("by", "app")
		q.Set("measure", "wire")
		q.Set("top", "8")
		var d struct {
			Names  []string
			Values [][]float64
		}
		if err := a.get("series", q, &d); err != nil {
			return fail(err)
		}
		for i, name := range d.Names {
			if i >= len(d.Values) {
				break
			}
			maxv := 0.0
			for _, x := range d.Values[i] {
				maxv = math.Max(maxv, x)
			}
			p.top = append(p.top, txt(name+"  "+fmtBps(maxv)), styled(blue, spark(d.Values[i], max(10, v.w-4), maxv)))
		}
	case "sources":
		var d struct {
			Stream  streamStatus `json:"stream"`
			Sources []struct {
				Exporter, Proto, Sampling, Status string
				LostPct                           float64 `json:"lost_pct"`
				TransitPct                        float64 `json:"transit_pct"`
				DropPct                           float64 `json:"drop_pct"`
				Pending                           int     `json:"pending"`
				ClockSkew                         float64 `json:"clock_skew_ns"`
				Issues                            []string
			}
		}
		if err := a.get("sources", nil, &d); err != nil {
			return fail(err)
		}
		var st struct {
			CounterSamples  uint64  `json:"counter_samples"`
			DroppedCounters uint64  `json:"dropped_counters"`
			RecordsPerSec   float64 `json:"records_per_sec"`
			Dropped         uint64
			DiskBytes       int64  `json:"disk_bytes"`
			DiskFree        int64  `json:"disk_free"`
			WriteErrors     uint64 `json:"write_errors"`
			LastError       string `json:"last_error"`
		}
		if err := a.get("status", nil, &st); err != nil {
			return fail(err)
		}
		p.top = []line{txt(fmt.Sprintf("%s: %.1f  %s: %d", a.t("self.ingest"), st.RecordsPerSec, a.t("self.dropped"), st.Dropped)), txt(a.t("self.disk") + ": " + fmtBytes(float64(st.DiskBytes)) + "  " + a.t("self.free") + ": " + fmtBytes(float64(st.DiskFree)))}
		p.top = append(p.top, txt(fmt.Sprintf("%s: %d  %s: %d  %s: %d", a.t("self.counter_samples"), st.CounterSamples, a.t("self.dropped_counters"), st.DroppedCounters, a.t("self.write_errors"), st.WriteErrors)))
		if d.Stream.Enforced && d.Stream.Configured {
			p.top = append(p.top, txt(a.t("stream.identity", "a", d.Stream.Agent, "s", fmt.Sprint(d.Stream.SubAgent))))
			p.top = append(p.top, txt(a.t("stream.rejected", "n", fmt.Sprint(d.Stream.Rejected))))
		}
		if st.WriteErrors > 0 {
			p.top = append(p.top, styled(red, fmt.Sprintf("%d: %s", st.WriteErrors, st.LastError)))
		}
		sec := section{cols: []column{{head: a.t("col.device")}, {head: a.t("col.proto"), width: 12}, {head: a.t("col.sampling"), width: 16}, {head: a.t("col.lost"), width: 8}, {head: a.t("col.status"), width: 12}}}
		for _, s := range d.Sources {
			r := row{key: s.Exporter, cells: []line{txt(s.Exporter), txt(s.Proto), txt(s.Sampling), txt(fmt.Sprintf("%.2f%%", s.LostPct)), txt(a.t("st." + s.Status))}}
			for _, issue := range s.Issues {
				loss := s.LostPct
				if issue == "loss" {
					loss = s.TransitPct
				}
				if issue == "agent_drops" {
					loss = s.DropPct
				}
				r.detail = append(r.detail, txt(a.t("issue."+issue, "a", s.Exporter, "p", fmt.Sprintf("%.1f%%", loss), "n", fmt.Sprint(s.Pending), "s", fmt.Sprintf("%.0f s", math.Abs(s.ClockSkew)/1e9))))
			}
			sec.rows = append(sec.rows, r)
		}
		p.sections = []section{sec}
	case "help":
		p.top = a.helpBox()
		p.top = append(p.top, txt(a.t("nav.account")+" / "+a.t("nav.cleanup")+": "+a.opt.URL))
	}
	if len(p.top) == 0 && (len(p.sections) == 0 || len(p.sections[0].rows) == 0) {
		p.top = append(p.top, styled(dim, a.t("empty.nodata")))
	}
	return p
}
