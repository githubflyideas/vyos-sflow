package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/collector"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func parseQuery(r *http.Request) (store.Query, error) {
	v := r.URL.Query()
	from, to, err := historyWindow(v, time.Now().UTC())
	q := store.Query{From: from, To: to}
	if err != nil {
		return q, err
	}
	if f := v.Get("f"); f != "" {
		if err := json.Unmarshal([]byte(f), &q.Filters); err != nil {
			return q, fmt.Errorf("bad filters: %w", err)
		}
		if len(q.Filters) > 20 {
			return q, errors.New("too many filters")
		}
	}
	if err := store.ValidateFilters(q.Filters); err != nil {
		return q, err
	}
	q.IncludeDup = v.Get("dup") == "1"
	q.ZoneOffset = zoneOffset(v.Get("tz"))
	return q, nil
}

// zoneOffset reads the viewer's offset from UTC, in minutes ahead of it,
// as the browser reports it. Anything outside the range real zones occupy
// is ignored rather than refused: a wrong offset would only shift the
// columns, and no chart is worth failing a request over.
func zoneOffset(s string) time.Duration {
	m, err := strconv.Atoi(s)
	if err != nil || m < -14*60 || m > 14*60 {
		return 0
	}
	return time.Duration(m) * time.Minute
}

func limitParam(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return def
	}
	if n > 1000 {
		n = 1000
	}
	return n
}

// ---------------------------------------------------------------- overview

type slicePart struct {
	Key  string `json:"key"`
	Wire uint64 `json:"wire"`
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		fail(w, err)
		return
	}
	out := map[string]any{"from": q.From.UnixMilli(), "to": q.To.UnixMilli()}
	tot, err := s.Store.Totals(q)
	if err != nil {
		fail(w, err)
		return
	}
	out["totals"] = tot
	nowQ := q
	nowQ.To = time.Now().UTC().Truncate(time.Minute)
	nowQ.From = nowQ.To.Add(-5 * time.Minute)
	if nt, err := s.Store.Totals(nowQ); err == nil {
		out["now_bps"] = float64(nt.Wire) * 8 / 300
	}

	se, err := s.Store.SeriesByApp(q, 8)
	if err != nil {
		fail(w, err)
		return
	}
	out["series"] = se
	peak, peakAt := 0.0, int64(0)
	for i := range se.Times {
		sum := 0.0
		for _, v := range se.Values {
			sum += v[i]
		}
		if sum > peak {
			peak, peakAt = sum, se.Times[i]
		}
	}
	out["peak_bps"], out["peak_at"] = peak, peakAt
	donut := func(dim string, keep int) []slicePart {
		rows, err := s.Store.TopN(q, dim, 20)
		if err != nil {
			return nil
		}
		var parts []slicePart
		var other uint64
		for i, t := range rows {
			if i < keep && t.Key != "" {
				parts = append(parts, slicePart{t.Key, t.Wire})
			} else {
				other += t.Wire
			}
		}
		if other > 0 {
			parts = append(parts, slicePart{"__other__", other})
		}
		return parts
	}
	out["proto"] = donut("proto", 2)
	if hosts, err := s.Store.TopN(q, "host", 10); err == nil {
		out["top_hosts"] = hosts
	}
	if apps, err := s.Store.TopN(q, "app", 10); err == nil {
		out["top_apps"] = apps
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- lists

func (s *Server) topn(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		fail(w, err)
		return
	}
	dim, by := r.URL.Query().Get("dim"), r.URL.Query().Get("by")
	if by == "" {
		by = "wire"
	}
	rows, err := s.Store.TopNBy(q, dim, limitParam(r, 66), by, r.URL.Query().Get("asc") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dim": dim, "rows": rows})
}

// ---------------------------------------------------------------- sources

type sourceOut struct {
	collector.SourceInfo
	Status string   `json:"status"` // ok, warn, bad
	Issues []string `json:"issues"`
	RecPS  float64  `json:"rec_per_sec"`
	// Blocked rows are ones whose datagrams are being dropped. A source
	// blocked a while ago stops appearing here at all, because nothing it
	// sends is decoded any more, which is why the page is also given the
	// whole blocklist separately.
	Blocked bool `json:"blocked,omitempty"`
}

var srcRate = struct {
	last map[string]uint64
	at   time.Time
	ps   map[string]float64
}{last: map[string]uint64{}, ps: map[string]float64{}}

func srcKey(si collector.SourceInfo) string {
	return si.Exporter + "|" + si.Proto + "|" + strconv.Itoa(int(si.Domain))
}

// sampleSources updates per-exporter record rates; called every few seconds.
func (s *Server) sampleSources(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dt := now.Sub(srcRate.at).Seconds()
	for _, si := range s.Col.Sources() {
		k := srcKey(si)
		if last, ok := srcRate.last[k]; ok && dt > 0.5 {
			r := float64(si.Records-last) / dt
			if old, ok := srcRate.ps[k]; ok {
				r = old*0.6 + r*0.4
			}
			srcRate.ps[k] = r
		}
		srcRate.last[k] = si.Records
	}
	srcRate.at = now
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	s.mu.Lock()
	var out []sourceOut
	var stale []map[string]any
	srcs := s.Col.Sources()
	active := map[string]bool{}
	for _, si := range srcs {
		if now.Sub(si.LastSeen) < 2*time.Minute {
			active[si.Exporter] = true
		}
	}
	for _, si := range srcs {
		o := sourceOut{SourceInfo: si, Status: "ok"}
		o.RecPS = srcRate.ps[srcKey(si)]
		issue := func(level, code string) {
			o.Issues = append(o.Issues, code)
			if level == "bad" || o.Status == "ok" {
				o.Status = level
			}
		}
		if now.Sub(si.LastSeen) > 2*time.Minute {
			issue("bad", "silent")
		}
		switch si.SamplingState {
		case "waiting":
			issue("warn", "waiting_sampling")
		case "assumed1":
			issue("warn", "assumed_unsampled")
		}
		if si.TransitPct >= 1 {
			issue("warn", "loss")
		}
		if si.DropPct >= 1 {
			issue("warn", "agent_drops")
		}
		if si.NoTemplate > 0 && si.Templates == 0 {
			issue("warn", "no_template")
		}
		if si.ClockSkew > 2*time.Minute || si.ClockSkew < -2*time.Minute {
			issue("warn", "clock_skew")
		}
		if si.DecodeErrors > 0 && si.DecodeErrors*100 > si.Packets {
			issue("warn", "decode_errors")
		}
		// silent for an hour while the same device sends otherwise (a
		// changed export version or domain): listed apart, not as a fault
		if now.Sub(si.LastSeen) > time.Hour && active[si.Exporter] {
			stale = append(stale, map[string]any{"exporter": si.Exporter, "proto": si.Proto, "domain": si.Domain, "last_seen": si.LastSeen})
			continue
		}
		out = append(out, o)
	}
	s.mu.Unlock()
	var ls []map[string]any
	for _, l := range s.Col.Listeners {
		ls = append(ls, map[string]any{"addr": l.Addr, "proto": l.Proto, "packets": l.Packets.Load(),
			"undecoded": l.Undecoded.Load(), "dropped": l.Dropped.Load(), "panics": l.Panics.Load(), "rcvbuf": l.RcvBuf})
	}
	res := map[string]any{"sources": out, "listeners": ls, "stale": stale, "stream": s.Col.StreamStatus()}
	if s.Col != nil && s.Col.Block != nil {
		bl := s.Col.Block.List()
		res["blocked"] = bl
		on := make(map[string]bool, len(bl))
		for _, b := range bl {
			on[b.Address] = true
		}
		for i := range out {
			out[i].Blocked = on[out[i].Exporter]
		}
		for _, st := range stale {
			if a, ok := st["exporter"].(string); ok && on[a] {
				st["blocked"] = true
			}
		}
	}

	writeJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------- misc

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	u := s.Store.Usage()
	s.rates.mu.Lock()
	rec, rows := s.rates.recPerSec, s.rates.rowRate
	s.rates.mu.Unlock()
	// Disk: what is used and free, and, once there is a day of data, how much
	// the kept days of detail will need at the current rate. Retention
	// deletes older data, so the disk does not fill as long as that fits.
	free := diskFree(s.DataDir)
	need := int64(-1)
	if !u.Oldest.IsZero() {
		age := time.Since(u.Oldest).Hours() / 24
		if age >= 1 && u.DiskBytes > 0 {
			need = int64(float64(u.DiskBytes) / age * float64(s.Store.RetentionDays()))
		}
	}
	warn := 0
	srcs := s.Col.Sources()
	for _, si := range srcs {
		if si.SamplingState == "waiting" || si.LostPct >= 1 || time.Since(si.LastSeen) > 2*time.Minute {
			warn++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.Version, "build": s.BuildKind, "tls": s.certInfo(), "now": time.Now().UnixMilli(), "settle_s": store.Settle().Seconds(), "demo": s.Demo, "uptime": int64(time.Since(s.Started).Seconds()),
		"records_per_sec": rec, "rows_per_sec": rows, "dropped": s.Pipe.Dropped.Load(), "dup_rows": s.Pipe.DupRows.Load(), "dedup_full": s.Pipe.DedupFull.Load(),
		"counter_samples": s.Pipe.CounterSamples.Load(), "dropped_counters": s.Pipe.DroppedCounters.Load(), "history_days": 2, "write_errors": s.Pipe.WriteErrs.Load(), "last_error": s.Pipe.LastError(),
		"disk_bytes": u.DiskBytes, "disk_free": free, "disk_need": need, "retention_days": s.Store.RetentionDays(), "oldest": u.Oldest.UnixMilli(),
		"hot_rows": u.HotRows, "segments": u.Segments, "segment_rows": u.SegmentRows,
		"dns_upstream": s.DNS.Upstream(), "dns_queries": s.DNS.Queries,
		"waiting": s.waitingOn(), "source_warnings": warn, "skewed": s.Col.Skewed.Load(), "go": runtime.Version(),
		"sources": len(srcs), "listen": s.Listen, "addrs": LocalIPs(), "stream": s.Col.StreamStatus(),
	})
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ IPs []string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		fail(w, err)
		return
	}
	if len(in.IPs) > 500 {
		in.IPs = in.IPs[:500]
	}
	out := map[string]string{}
	var ask []string
	for _, ip := range in.IPs {
		_, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		ask = append(ask, ip)
	}
	for ip, n := range s.DNS.Lookup(ask, 800*time.Millisecond) {
		out[ip] = n
	}
	writeJSON(w, http.StatusOK, out)
}

// series: a measure over time split by one dimension (top values and the
// rest), for the detail charts.
func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		fail(w, err)
		return
	}
	v := r.URL.Query()
	measure := v.Get("measure")
	if measure == "" {
		measure = "wire"
	}
	top, _ := strconv.Atoi(v.Get("top"))
	se, err := s.Store.SeriesBy(q, v.Get("by"), measure, top)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// certDownload serves the certificate the web UI is using, so it can be
// checked or added to a trust store. It needs no sign-in: the same bytes
// are handed to anyone who opens the page, and somebody who does not
// trust the certificate yet cannot sign in to fetch it.
func (s *Server) certDownload(w http.ResponseWriter, r *http.Request) {
	if s.Cert == nil || !s.Cert.SelfSigned || s.Cert.CertPath == "" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(s.Cert.CertPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="traffic66.crt"`)
	w.Write(b)
}

// certInfo describes the certificate the web UI is served with, so the
// settings page can show the fingerprint a browser warned about. nil over
// plain HTTP.
func (s *Server) certInfo() any {
	if s.Cert == nil {
		return nil
	}
	return map[string]any{
		"self_signed": s.Cert.SelfSigned,
		"names":       s.Cert.Names,
		"fingerprint": s.Cert.Fingerprint,
		"expires":     s.Cert.NotAfter.UTC().Format("2006-01-02"),
	}
}

// putIface names, tags or marks one interface as the default.
