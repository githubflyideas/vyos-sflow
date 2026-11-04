package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Filter is one condition: Field equals (or, with Neg, does not equal) Value.
type Filter struct {
	Field string `json:"f"`
	Neg   bool   `json:"neg"`
	Value string `json:"v"`
}

// Query selects flow slices in [From, To).
type Query struct {
	From, To   time.Time
	Filters    []Filter
	IncludeDup bool
	// ZoneOffset is how far the viewer's clock is ahead of UTC, as the
	// browser reports it. Buckets are cut on that clock, not on UTC's: a
	// day cut at UTC midnight falls at nine in the morning in Tokyo, so
	// every day of a Japanese installation was split across two columns
	// and neither was a day's traffic. It is zero for a viewer on UTC and
	// for callers that do not care, which is the behaviour this had.
	//
	// It is one offset for the whole range rather than a named zone: the
	// boundaries of a range are already the browser's to decide here (see
	// the billing endpoint), and a zone database in the binary would be a
	// large dependency for the hour that a range spanning a daylight-saving
	// change would move one boundary by.
	ZoneOffset time.Duration
}

// localTrunc truncates t down to a multiple of step on the viewer's clock.
func (q Query) localTrunc(t time.Time, step time.Duration) time.Time {
	return t.UTC().Add(q.ZoneOffset).Truncate(step).Add(-q.ZoneOffset)
}

// bucketSQL is the expression that puts a row in its bucket, on the same
// clock localTrunc uses, so the rows and the column they are counted into
// agree.
func (q Query) bucketSQL(step time.Duration) string {
	secs := int64(step.Seconds())
	off := int64(q.ZoneOffset.Seconds())
	if off == 0 {
		return fmt.Sprintf("time_bucket(INTERVAL '%d seconds', ts)", secs)
	}
	return fmt.Sprintf("time_bucket(INTERVAL '%d seconds', ts + INTERVAL '%d seconds') - INTERVAL '%d seconds'", secs, off, off)
}

func (q Query) span() time.Duration { return q.To.Sub(q.From) }

var protoByName = map[string]int{"icmp": 1, "igmp": 2, "ipip": 4, "tcp": 6, "udp": 17, "ipv6": 41, "gre": 47, "esp": 50, "ah": 51, "icmpv6": 58, "ospf": 89, "sctp": 132}

func v4range(p netip.Prefix) (uint32, uint32, bool) {
	if !p.Addr().Is4() {
		return 0, 0, false
	}
	a := p.Masked().Addr().As4()
	lo := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
	hi := lo | (uint32(1)<<(32-p.Bits()) - 1)
	if p.Bits() == 0 {
		hi = 0xFFFFFFFF
	}
	return lo, hi, true
}

// oneDevice: the query keeps to one device or interface. Duplicates are
// left out so that traffic reported by two devices counts once; within one
// device they are its own traffic and count.
func (q Query) oneDevice() bool {
	for _, f := range q.Filters {
		if !f.Neg && (f.Field == "exporter" || strings.HasPrefix(f.Field, "iface")) {
			return true
		}
	}
	return false
}

// where builds the WHERE clause and its arguments.
func (q Query) where() (string, []any, error) {
	if err := validateFilters(q.Filters, true); err != nil {
		return "", nil, err
	}
	conds := []string{"ts >= ?", "ts < ?"}
	args := []any{q.From.UTC(), q.To.UTC()}
	if !q.IncludeDup && !q.oneDevice() {
		conds = append(conds, "NOT dup")
	}
	for _, f := range q.Filters {
		var c string
		v := strings.TrimSpace(f.Value)
		switch f.Field {
		case "ip":
			cols := map[string][]string{"ip": {"client", "server"}, "client": {"client"}, "server": {"server"}}[f.Field]
			n4 := map[string][]string{"ip": {"cli4", "srv4"}, "client": {"cli4"}, "server": {"srv4"}}[f.Field]
			var parts []string
			if p, err := netip.ParsePrefix(v); err == nil && !p.IsSingleIP() {
				lo, hi, ok := v4range(p)
				if !ok {
					return "", nil, fmt.Errorf("IPv6 prefixes are not supported in filters yet: %s", v)
				}
				for _, col := range n4 {
					parts = append(parts, col+" BETWEEN ? AND ?")
					args = append(args, lo, hi)
				}
			} else {
				if a, err := netip.ParseAddr(strings.TrimSuffix(v, "/32")); err == nil {
					v = a.Unmap().String()
				}
				for _, col := range cols {
					parts = append(parts, col+" = ?")
					args = append(args, v)
				}
			}
			c = "(" + strings.Join(parts, " OR ") + ")"
		case "src", "dst":
			// one address as the sender, or as the receiver, of the bytes:
			// what went to an address and what came from it
			a, err := netip.ParseAddr(v)
			if err != nil {
				return "", nil, fmt.Errorf("bad address %q", v)
			}
			c = f.Field + " = ?"
			args = append(args, a.Unmap().String())
		case "port":
			port := v
			if i := strings.IndexByte(v, '/'); i > 0 {
				port = v[:i]
				pn := protoByName[strings.ToLower(v[i+1:])]
				if pn > 0 {
					c = "(svc_port = ? AND proto = ?)"
					pv, _ := strconv.Atoi(port)
					args = append(args, pv, pn)
					break
				}
			}
			pv, err := strconv.Atoi(port)
			if err != nil {
				return "", nil, fmt.Errorf("bad port %q", v)
			}
			c = "svc_port = ?"
			args = append(args, pv)
		case "app":
			c = "app = ?"
			args = append(args, v)
		case "proto":
			n, ok := protoByName[strings.ToLower(v)]
			if !ok {
				x, err := strconv.Atoi(v)
				if err != nil {
					return "", nil, fmt.Errorf("bad protocol %q", v)
				}
				n = x
			}
			c = "proto = ?"
			args = append(args, n)
		case "exporter":
			c = "exporter = ?"
			args = append(args, v)
		case "iface", "iface_in", "iface_out":
			// exporter/ifindex: either direction, or only what came in
			// through it, or only what went out through it
			i := strings.LastIndexByte(v, '/')
			if i <= 0 {
				return "", nil, fmt.Errorf("bad interface %q", v)
			}
			n, err := strconv.Atoi(v[i+1:])
			if err != nil {
				return "", nil, fmt.Errorf("bad interface %q", v)
			}
			switch f.Field {
			case "iface_in":
				c = "(exporter = ? AND in_if = ?)"
				args = append(args, v[:i], n)
			case "iface_out":
				c = "(exporter = ? AND out_if = ?)"
				args = append(args, v[:i], n)
			default:
				c = "(exporter = ? AND (in_if = ? OR out_if = ?))"
				args = append(args, v[:i], n, n)
			}
		default:
			return "", nil, fmt.Errorf("unknown filter field %q", f.Field)
		}
		if f.Neg {
			c = "NOT " + c
		}
		conds = append(conds, c)
	}
	return strings.Join(conds, " AND "), args, nil
}

// TopRow is one Top-N entry.
type TopRow struct {
	Key   string `json:"key"`
	Bytes uint64 `json:"bytes"`
	Wire  uint64 `json:"wire"`
	Pkts  uint64 `json:"pkts"`
	Flows uint64 `json:"flows"`
}

// Supported rankings are basic endpoint, port-service and IP protocol totals.
type dimDef struct{ expr, rollup string }

var dims = map[string]dimDef{
	"host":   {expr: "unnest(list_distinct([client, server]))"},
	"client": {expr: "client"},
	"server": {expr: "server"},
	"app":    {expr: "app", rollup: "app"},
	"proto":  {expr: "CAST(proto AS VARCHAR)", rollup: "proto"},
}
var topOrder = map[string]string{"wire": "wire", "bytes": "bytes", "pkts": "pkts", "flows": "flows"}

func (s *Store) TopN(q Query, dim string, limit int) ([]TopRow, error) {
	return s.TopNBy(q, dim, limit, "wire")
}

// Host totals count traffic involving either endpoint, once per distinct host
// per flow. Summing host rows therefore is not a link traffic total.
func (s *Store) TopNBy(q Query, dim string, limit int, by string, asc ...bool) ([]TopRow, error) {
	d, ok := dims[dim]
	if !ok {
		return nil, fmt.Errorf("unknown dimension %q", dim)
	}
	order, ok := topOrder[by]
	if !ok {
		return nil, fmt.Errorf("unknown ranking %q", by)
	}
	if limit <= 0 || limit > 1000 {
		limit = 66
	}
	direction := "DESC"
	if len(asc) > 0 && asc[0] {
		direction = "ASC"
	}
	where, args, err := q.where()
	if err != nil {
		return nil, err
	}
	inner := fmt.Sprintf("SELECT %s AS k, bytes, wire, pkts, flows FROM %s WHERE %s", d.expr, s.Source(q.From, q.To), where)
	if d.rollup != "" && s.rollupOK(q, false) {
		inner = "SELECT val AS k, bytes, wire, pkts, flows FROM r_dim WHERE ts>=? AND ts<? AND dim=?"
		args = []any{q.From.UTC(), q.To.UTC(), d.rollup}
	}
	rows, err := s.DB.Query(fmt.Sprintf("SELECT k, coalesce(sum(bytes),0), coalesce(sum(wire),0), coalesce(sum(pkts),0), coalesce(sum(flows),0) FROM (%s) GROUP BY k ORDER BY sum(%s) %s, k LIMIT %d", inner, order, direction, limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopRow{}
	for rows.Next() {
		var row TopRow
		if err := rows.Scan(&row.Key, &row.Bytes, &row.Wire, &row.Pkts, &row.Flows); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type Series struct {
	Step   int64       `json:"step"` // seconds
	Times  []int64     `json:"times"`
	Names  []string    `json:"names"`
	Values [][]float64 `json:"values"` // [series][time] bits (or packets) per second
	// SeriesBy only: each series' total over the range (bytes or packets),
	// and how many distinct values there were
	Totals   []float64 `json:"totals,omitempty"`
	Distinct int       `json:"distinct,omitempty"`
	// SettledTo is the moment up to which the data is complete, in
	// milliseconds. Buckets after it are drawn, because leaving the newest
	// data off a live chart is its own kind of lie, but they are still
	// filling: the page marks them rather than letting a half-counted
	// minute look like a fall in traffic. Zero when the whole range is
	// settled, which is every range that does not reach up to now.
	SettledTo int64 `json:"settled_to,omitempty"`
	// DataFrom is when the stored data begins, in milliseconds, when that
	// is inside the range. Buckets that end before it are not "no traffic"
	// but "not collecting yet": the page leaves them out instead of
	// drawing a flat line at zero.
	DataFrom int64 `json:"data_from,omitempty"`
}

// bucketSecs is how many seconds of bucket i the data can cover, the
// divisor that turns its bytes into a rate. A bucket is cut short at the
// start by the beginning of the data (an installation that started
// mid-bucket) and at the end by the end of the range. Rows are kept per
// minute, so a range that ends in the past covers its last minute whole:
// that minute's row is in it. A range that ends now only covers up to now.
func bucketSecs(start time.Time, i int, step time.Duration, to, dataFrom time.Time) float64 {
	b := start.Add(time.Duration(i) * step)
	e := b.Add(step)
	end := to
	if time.Since(to) > 2*time.Minute {
		if c := to.Truncate(time.Minute); c.Before(to) {
			end = c.Add(time.Minute)
		}
	}
	if e.After(end) {
		e = end
	}
	if dataFrom.After(b) {
		b = dataFrom
	}
	return max(1, e.Sub(b).Seconds())
}

// dataFromMark is the DataFrom of a series starting at start, or 0.
func dataFromMark(start, dataFrom time.Time) int64 {
	if dataFrom.IsZero() || !dataFrom.After(start) {
		return 0
	}
	return dataFrom.UnixMilli()
}

// settle is how long the data of a moment keeps arriving after it. The
// pipeline measures it from the flows themselves: about a second for sFlow,
// up to the active timeout for NetFlow and IPFIX (capped at 2 minutes).
var settleNs atomic.Int64

func init() { settleNs.Store(int64(time.Minute)) }

// SetSettle records the current delay of incoming data.
func SetSettle(d time.Duration) { settleNs.Store(int64(d)) }

// Settle is the current delay of incoming data.
func Settle() time.Duration { return time.Duration(settleNs.Load()) }

// settledTo is the moment up to which the data of a range that ends now is
// complete. Rows are kept per minute: with data that arrives at once
// (sFlow) the current minute is complete as far as it goes; with delayed
// data (NetFlow, IPFIX) only the minutes older than the settle delay are.
// Summaries are written once a minute, a minute later.
//
// A range that ends in the past is complete throughout, and gets back the
// end it was given.
func settledTo(to time.Time, rollup bool) time.Time {
	now, s := time.Now(), Settle()
	if rollup {
		s += time.Minute
	}
	if now.Sub(to) > s+2*time.Minute {
		return to // a range in the past
	}
	e := now
	if s > 5*time.Second {
		e = now.Add(-s).Truncate(time.Minute)
	}
	if e.Before(to) {
		return e
	}
	return to
}

// unsettledMark reports the boundary to put on a series, in milliseconds,
// or 0 when there is nothing to mark because the range is complete.
func unsettledMark(settled, to time.Time) int64 {
	if !settled.Before(to) {
		return 0
	}
	return settled.UnixMilli()
}

// bucketCount is the number of buckets from start up to to; a last bucket
// that would cover less than 15 seconds is left out.
func bucketCount(start, to time.Time, step time.Duration) int {
	n := int(to.Sub(start)/step) + 1
	if n > 1 && to.Sub(start.Add(time.Duration(n-1)*step)) < 15*time.Second {
		n--
	}
	return n
}

// StepFor picks a bucket size giving at most ~300 points.
func StepFor(span time.Duration) time.Duration {
	for _, s := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour, 24 * time.Hour} {
		if span/s <= 300 {
			return s
		}
	}
	return 24 * time.Hour
}

// SeriesBy returns bits/s over time split by app (top k, rest as "__other__").
func (s *Store) SeriesByApp(q Query, k int) (*Series, error) {
	step := StepFor(q.span())
	rollup := s.rollupOK(q, false)
	settled := settledTo(q.To, rollup)
	var rows *sql.Rows
	var err error
	bucket := q.bucketSQL(step)
	if rollup {
		sqlq, pre, rerr := s.rollups(q, fmt.Sprintf(`SELECT %s AS b, app, sum(wire) FROM {r_ts} WHERE ts >= ? AND ts < ? GROUP BY 1,2`, bucket))
		if rerr != nil {
			return nil, rerr
		}
		rows, err = s.DB.Query(sqlq, append(pre, q.From.UTC(), q.To.UTC())...)
	} else {
		where, args, werr := q.where()
		if werr != nil {
			return nil, werr
		}
		rows, err = s.DB.Query(fmt.Sprintf(`SELECT %s AS b, app, sum(wire) FROM %s WHERE %s GROUP BY 1,2`, bucket, s.Source(q.From, q.To), where), args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pt struct {
		t   time.Time
		app string
		v   float64
	}
	var pts []pt
	tot := map[string]float64{}
	for rows.Next() {
		var p pt
		var w uint64
		if err := rows.Scan(&p.t, &p.app, &w); err != nil {
			return nil, err
		}
		p.v = float64(w)
		pts = append(pts, p)
		tot[p.app] += p.v
	}
	names := make([]string, 0, len(tot))
	for n := range tot {
		names = append(names, n)
	}
	// equal totals by name, so the legend keeps its order between redraws
	sort.Slice(names, func(i, j int) bool {
		if tot[names[i]] != tot[names[j]] {
			return tot[names[i]] > tot[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > k {
		names = append(names[:k:k], "__other__")
	}
	idx := map[string]int{}
	for i, n := range names {
		idx[n] = i
	}
	start := q.localTrunc(q.From, step)
	n := bucketCount(start, q.To, step)
	se := &Series{Step: int64(step.Seconds()), Names: names, Values: make([][]float64, len(names))}
	se.SettledTo = unsettledMark(settled, q.To)
	dataFrom := s.DataStart()
	se.DataFrom = dataFromMark(start, dataFrom)
	for i := 0; i < n; i++ {
		se.Times = append(se.Times, start.Add(time.Duration(i)*step).UnixMilli())
	}
	for i := range se.Values {
		se.Values[i] = make([]float64, n)
	}
	for _, p := range pts {
		ti := int(p.t.UTC().Sub(start) / step)
		if ti < 0 || ti >= n {
			continue
		}
		si, ok := idx[p.app]
		if !ok {
			si = idx["__other__"]
		}
		se.Values[si][ti] += p.v * 8 / bucketSecs(start, ti, step, q.To, dataFrom)
	}
	return se, nil
}

// TotalSeries returns total bits/s over time (one series), used for
// baselines. It shares SeriesByApp's bucketing.
func (s *Store) TotalSeries(q Query) (*Series, error) {
	se, err := s.SeriesByApp(q, 1<<30)
	if err != nil {
		return nil, err
	}
	tot := make([]float64, len(se.Times))
	for _, v := range se.Values {
		for i, x := range v {
			tot[i] += x
		}
	}
	return &Series{Step: se.Step, Times: se.Times, Names: []string{"total"}, Values: [][]float64{tot}}, nil
}

// Totals are headline numbers for a range.
type Totals struct {
	Bytes uint64 `json:"bytes"`
	Wire  uint64 `json:"wire"`
	Pkts  uint64 `json:"pkts"`
	Flows uint64 `json:"flows"`
	Hosts uint64 `json:"hosts"` // internal addresses seen
	Peers uint64 `json:"peers"` // external addresses seen

}

func (s *Store) Totals(q Query) (Totals, error) {
	var t Totals
	if s.rollupOK(q, false) {
		one := func(sqlq string, dst ...any) error {
			sqlq, pre, err := s.rollups(q, sqlq)
			if err != nil {
				return err
			}
			return s.DB.QueryRow(sqlq, append(pre, q.From.UTC(), q.To.UTC())...).Scan(dst...)
		}
		if err := one(`SELECT coalesce(sum(bytes),0), coalesce(sum(wire),0), coalesce(sum(pkts),0), coalesce(sum(flows),0) FROM {r_ts} WHERE ts >= ? AND ts < ?`,
			&t.Bytes, &t.Wire, &t.Pkts, &t.Flows); err != nil {
			return t, err
		}
		if err := one(`SELECT count(DISTINCT ip) FILTER (WHERE internal), count(DISTINCT ip) FILTER (WHERE NOT internal) FROM {r_host} WHERE ts >= ? AND ts < ?`, &t.Hosts, &t.Peers); err != nil {
			return t, err
		}
		return t, nil
	}

	where, args, err := q.where()
	if err != nil {
		return t, err
	}
	src := s.Source(q.From, q.To)
	err = s.DB.QueryRow(fmt.Sprintf(`SELECT coalesce(sum(bytes),0), coalesce(sum(wire),0), coalesce(sum(pkts),0), coalesce(sum(flows),0)
		FROM %s WHERE %s`, src, where), args...).
		Scan(&t.Bytes, &t.Wire, &t.Pkts, &t.Flows)
	if err != nil {
		return t, err
	}
	err = s.DB.QueryRow(s.hostCounts(q, where), args...).Scan(&t.Hosts, &t.Peers)
	return t, err
}

// hostCounts counts the internal addresses on either side, each once (a
// host is often a client in one conversation and a server in another),
// and the external ones. They are counted by grouping them, which spills
// to disk where count(DISTINCT) over a week of addresses ran out of memory.
func (s *Store) hostCounts(q Query, where string) string {
	return fmt.Sprintf(`WITH x AS NOT MATERIALIZED (SELECT client, server, cli_int, srv_int FROM %s WHERE %s)
		SELECT (SELECT count(*) FROM (SELECT client FROM x WHERE cli_int UNION SELECT server FROM x WHERE srv_int)),
			(SELECT count(*) FROM (SELECT CASE WHEN NOT srv_int THEN server ELSE client END FROM x WHERE NOT srv_int OR NOT cli_int GROUP BY 1))`,
		s.Source(q.From, q.To), where)
}

// ValidateFilters rejects removed dimensions before any storage operation.
func ValidateFilters(filters []Filter) error { return validateFilters(filters, false) }

// validateFilters also lets through, where internal is set, the interface
// filters the server adds itself (inbound and outbound on the overview);
// a request cannot ask for them.
func validateFilters(filters []Filter, internal bool) error {
	for _, f := range filters {
		switch f.Field {
		case "ip", "src", "dst", "port", "proto", "app", "exporter":
		case "iface", "iface_in", "iface_out":
			if !internal {
				return fmt.Errorf("unknown filter field %q", f.Field)
			}
		default:
			return fmt.Errorf("unknown filter field %q", f.Field)
		}
	}
	return nil
}

// Record is one flow record for the records page.
type Record struct {
	TS      int64   `json:"ts"`
	Client  string  `json:"client"`
	Server  string  `json:"server"`
	CPort   uint16  `json:"cport"`
	SvcPort uint16  `json:"port"`
	Proto   uint8   `json:"proto"`
	App     string  `json:"app"`
	Wire    uint64  `json:"wire"`
	Pkts    uint64  `json:"pkts"`
	InIf    uint32  `json:"in_if"`
	OutIf   uint32  `json:"out_if"`
	Flags   uint8   `json:"tcp_flags"`
	Mult    float32 `json:"mult"`
}

// Records returns the newest flow records, at most limit.
func (s *Store) Records(q Query, limit int) ([]Record, error) {
	where, args, err := q.where()
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(fmt.Sprintf(`SELECT ts, client, server, CASE WHEN client = src THEN sport ELSE dport END, svc_port, proto, app,
		wire, pkts, in_if, out_if, tcp_flags, mult FROM %s WHERE %s ORDER BY ts DESC, wire DESC LIMIT %d`, s.Source(q.From, q.To), where, limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var ts time.Time
		if err := rows.Scan(&ts, &r.Client, &r.Server, &r.CPort, &r.SvcPort, &r.Proto, &r.App, &r.Wire, &r.Pkts, &r.InIf, &r.OutIf, &r.Flags, &r.Mult); err != nil {
			return nil, err
		}
		r.TS = ts.UnixMilli()
		out = append(out, r)
	}
	return out, rows.Err()
}

// BusiestIface is the interface most of the traffic in q went in or out
// through, as "exporter/ifindex": the one sFlow samples on.
func (s *Store) BusiestIface(q Query) (string, error) {
	where, args, err := q.where()
	if err != nil {
		return "", err
	}
	var exp string
	var idx uint32
	err = s.DB.QueryRow(fmt.Sprintf(`SELECT exporter, i FROM (
			SELECT exporter, in_if AS i, wire FROM %[1]s WHERE %[2]s AND in_if <> 0
			UNION ALL SELECT exporter, out_if, wire FROM %[1]s WHERE %[2]s AND out_if <> 0)
		GROUP BY 1, 2 ORDER BY sum(wire) DESC LIMIT 1`, s.Source(q.From, q.To), where), append(append([]any{}, args...), args...)...).Scan(&exp, &idx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return exp + "/" + strconv.FormatUint(uint64(idx), 10), nil
}
