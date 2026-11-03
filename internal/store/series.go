package store

import (
	"fmt"
	"sort"
	"time"
)

// Series dimensions: what one chart line stands for. key is the SQL
// expression of the value, cond limits the rows (true for all). rollup, when
// set, answers long ranges from the hourly summaries: the query selects
// (bucket, key, wire, pkts) for the given bucket expression and range.
type seriesDim struct {
	key, cond string
	rollup    func(bucket string) string
	hourly    bool // the rollup has one row per hour: no finer buckets
}

var seriesDims = map[string]seriesDim{
	"host": {key: "unnest(list_distinct([client, server]))", cond: "true"},
	"app": {key: "app", cond: "true", rollup: func(b string) string {
		return `SELECT ` + b + `,app,sum(wire),sum(pkts) FROM {r_ts} WHERE ts>=? AND ts<? GROUP BY 1,2`
	}},
	"proto": {key: "CAST(proto AS VARCHAR)", cond: "true", hourly: true, rollup: func(b string) string {
		return `SELECT ` + b + `,val,sum(wire),sum(pkts) FROM {r_dim} WHERE ts>=? AND ts<? AND dim='proto' GROUP BY 1,2`
	}},
}

// SeriesBy returns a measure over time split by a dimension: bits per
// second ("wire") or packets per second ("pkts"), for the top k values; the
// rest is summed as "__other__". Distinct estimates how many values there were.
func (s *Store) SeriesBy(q Query, by, measure string, k int) (*Series, error) {
	d, ok := seriesDims[by]
	if !ok {
		return nil, fmt.Errorf("unknown series %q", by)
	}
	if measure != "wire" && measure != "pkts" {
		return nil, fmt.Errorf("unknown measure %q", measure)
	}
	if k <= 0 || k > 20 {
		k = 8
	}
	// decide before the start is rounded down to a bucket: rounding makes a
	// 6-hour range a little longer, which must not switch it to the summaries
	rollup := d.rollup != nil && s.rollupOK(q, by == "host")
	step := StepFor(q.span())
	if rollup && d.hourly && step < time.Hour {
		// hourly summaries in 5-minute buckets would show one spike an hour
		step = time.Hour
	}
	settled := settledTo(q.To, rollup)
	bucket := q.bucketSQL(step)
	var inner string
	var args []any
	if rollup {
		r, pre, err := s.rollups(q, d.rollup(bucket))
		if err != nil {
			return nil, err
		}
		inner = r
		args = append(pre, q.From.UTC(), q.To.UTC())
	} else {
		where, wargs, err := q.where()
		if err != nil {
			return nil, err
		}
		// the rows themselves: grouped by bucket and value before the top
		// values are known, a week by client is a group for nearly every row
		inner = fmt.Sprintf(`SELECT %s, %s, wire, pkts FROM %s WHERE %s AND %s`, bucket, d.key, s.Source(q.From, q.To), where, d.cond)
		args = wargs
	}
	m := "w"
	if measure == "pkts" {
		m = "p"
	}
	// keep the top k values apart, everything else as one series, and count
	// the values
	// b is read again by each step rather than kept: kept, a week of
	// buckets by client is more than the memory there is. The values are
	// counted approximately, in a few kilobytes, where counting them
	// exactly holds every one of them.
	sqlq := fmt.Sprintf(`WITH b AS NOT MATERIALIZED (SELECT * FROM (%s) AS x(t, k, w, p)),
			top AS (SELECT k FROM b GROUP BY k ORDER BY sum(%s) DESC LIMIT %d)
		SELECT t, CASE WHEN k IN (SELECT k FROM top) THEN k ELSE '__other__' END AS kk, CAST(sum(%s) AS DOUBLE), (SELECT approx_count_distinct(k) FROM b)
		FROM b GROUP BY 1,2`, inner, m, k, m)
	rows, err := s.DB.Query(sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pt struct {
		t time.Time
		k string
		v float64
	}
	var pts []pt
	tot := map[string]float64{}
	distinct := 0
	for rows.Next() {
		var p pt
		var v float64
		if err := rows.Scan(&p.t, &p.k, &v, &distinct); err != nil {
			return nil, err
		}
		p.v = v
		pts = append(pts, p)
		tot[p.k] += v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tot))
	for n := range tot {
		if n != "__other__" {
			names = append(names, n)
		}
	}
	// equal totals by name, so the legend keeps its order between redraws
	sort.Slice(names, func(i, j int) bool {
		if tot[names[i]] != tot[names[j]] {
			return tot[names[i]] > tot[names[j]]
		}
		return names[i] < names[j]
	})
	if _, ok := tot["__other__"]; ok {
		names = append(names, "__other__")
	}
	idx := map[string]int{}
	for i, n := range names {
		idx[n] = i
	}
	start := q.localTrunc(q.From, step)
	n := bucketCount(start, q.To, step)
	se := &Series{Step: int64(step.Seconds()), Names: names, Values: make([][]float64, len(names)), Distinct: distinct}
	se.SettledTo = unsettledMark(settled, q.To)
	dataFrom := s.DataStart()
	se.DataFrom = dataFromMark(start, dataFrom)
	se.Totals = make([]float64, len(names))
	for i, nm := range names {
		se.Totals[i] = tot[nm]
	}
	for i := 0; i < n; i++ {
		se.Times = append(se.Times, start.Add(time.Duration(i)*step).UnixMilli())
	}
	for i := range se.Values {
		se.Values[i] = make([]float64, n)
	}
	f := 8.0 // bytes to bits
	if measure == "pkts" {
		f = 1
	}
	for _, p := range pts {
		ti := int(p.t.UTC().Sub(start) / step)
		if ti < 0 || ti >= n {
			continue
		}
		se.Values[idx[p.k]][ti] += p.v * f / bucketSecs(start, ti, step, q.To, dataFrom)
	}
	return se, nil
}
