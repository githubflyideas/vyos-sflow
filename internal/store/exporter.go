package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PurgeExporter deletes every flow record one device exported: from the
// table of recent rows and from the hourly files, which are rewritten
// without them. It is for a device whose data should not be there at all,
// such as one that sent forged or broken exports.
//
// The summaries have no device column, so the device cannot be taken out
// of them. They are only a shortcut for long ranges, and the flow records
// cover all the history kept; the hours the device had data in are noted,
// and queries over them read the flow records instead (see summariesOK).
func (s *Store) PurgeExporter(exporter string) (PurgeSize, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	var done PurgeSize
	for _, a := range []interface{ Flush() error }{s.appHot, s.appTS, s.appHost, s.appDim} {
		if err := a.Flush(); err != nil {
			return done, err
		}
	}
	var first, last time.Time
	var n int64
	src := s.Source(time.Time{}, time.Now().Add(24*time.Hour))
	if err := s.DB.QueryRow(`SELECT count(*), coalesce(min(ts), TIMESTAMP '1970-01-01'), coalesce(max(ts), TIMESTAMP '1970-01-01') FROM `+src+` WHERE exporter = ?`, exporter).Scan(&n, &first, &last); err != nil {
		return done, err
	}
	if n == 0 {
		return done, nil
	}
	if err := s.noteUnsummarised(first, last); err != nil {
		return done, err
	}
	defer s.loadSegments()
	res, err := s.DB.Exec(`DELETE FROM hot WHERE exporter = ?`, exporter)
	if err != nil {
		return done, err
	}
	if _, err := s.DB.Exec(`UPDATE hot SET dup = false WHERE rowid IN (SELECT rowid FROM (SELECT rowid, ` + promoteDup + ` AS keep FROM hot WINDOW k AS (PARTITION BY ` + dupKey + `)) WHERE keep = false)`); err != nil {
		return done, err
	}
	hot, _ := res.RowsAffected()
	done.Rows += hot
	s.segMu.RLock()
	segs := append([]segment(nil), s.segCache...)
	s.segMu.RUnlock()
	for _, g := range segs {
		dropped, removed, err := s.dropFromSegment(g, exporter)
		if err != nil {
			return done, fmt.Errorf("purge %s: %w", g.path, err)
		}
		done.Rows += dropped
		if dropped > 0 {
			done.Files++
		}
		_ = removed
	}
	s.dsMu.Lock()
	s.dsTS, s.dsAt = time.Time{}, time.Time{}
	s.dsMu.Unlock()
	return done, nil
}

// dropFromSegment rewrites one hourly file without a device's records, or
// removes it when nothing else is left.
func (s *Store) dropFromSegment(g segment, exporter string) (dropped int64, removed bool, err error) {
	src := "'" + sqlPath(g.path) + "'"
	var total, keep int64
	var mn, mx time.Time
	if err := s.DB.QueryRow(`SELECT count(*), count(*) FILTER (WHERE exporter <> ?), coalesce(min(ts) FILTER (WHERE exporter <> ?), TIMESTAMP '1970-01-01'), coalesce(max(ts) FILTER (WHERE exporter <> ?), TIMESTAMP '1970-01-01') FROM read_parquet(`+src+`)`,
		exporter, exporter, exporter).Scan(&total, &keep, &mn, &mx); err != nil {
		return 0, false, err
	}
	if keep == total {
		return 0, false, nil
	}
	if keep == 0 {
		if err := os.Remove(g.path); err != nil && !os.IsNotExist(err) {
			return 0, false, err
		}
		if _, err := s.DB.Exec(`DELETE FROM segments WHERE path = ?`, g.path); err != nil {
			return 0, false, err
		}
		os.Remove(filepath.Dir(g.path))
		return total, true, nil
	}
	tmp := g.path + ".purge"
	q := fmt.Sprintf(`COPY (SELECT * REPLACE (`+promoteDup+` AS dup) FROM read_parquet(%s) WHERE exporter <> ? WINDOW k AS (PARTITION BY `+dupKey+`) ORDER BY ts, client) TO '%s' (FORMAT parquet, COMPRESSION zstd, ROW_GROUP_SIZE 122880)`, src, sqlPath(tmp))
	if _, err := s.DB.Exec(q, exporter); err != nil {
		os.Remove(tmp)
		return 0, false, err
	}
	if err := os.Rename(tmp, g.path); err != nil {
		os.Remove(tmp)
		return 0, false, err
	}
	if _, err := s.DB.Exec(`UPDATE segments SET min_ts = ?, max_ts = ?, rows = ? WHERE path = ?`, mn, mx, keep, g.path); err != nil {
		return 0, false, err
	}
	return total - keep, false, nil
}

const unsummarisedKey = "unsummarised"

// noteUnsummarised widens the window whose summaries still hold a deleted
// device's traffic, by whole hours.
func (s *Store) noteUnsummarised(first, last time.Time) error {
	from, to := first.UTC().Truncate(time.Hour), last.UTC().Truncate(time.Hour).Add(time.Hour)
	if f, t, ok := s.unsummarised(); ok {
		if f.Before(from) {
			from = f
		}
		if t.After(to) {
			to = t
		}
	}
	return s.SetMeta(unsummarisedKey, from.Format(time.RFC3339)+" "+to.Format(time.RFC3339))
}

func (s *Store) unsummarised() (from, to time.Time, ok bool) {
	var a, b string
	if _, err := fmt.Sscan(s.Meta(unsummarisedKey), &a, &b); err != nil {
		return
	}
	f, err1 := time.Parse(time.RFC3339, a)
	t, err2 := time.Parse(time.RFC3339, b)
	return f, t, err1 == nil && err2 == nil
}

// summariesOK reports whether the summaries can answer for q: not when it
// overlaps hours a deleted device still counts in.
func (s *Store) summariesOK(q Query) bool {
	f, t, ok := s.unsummarised()
	return !ok || !q.From.Before(t) || !q.To.After(f)
}

// StoredExporter is one device with flow records kept.
type StoredExporter struct {
	Exporter string    `json:"exporter"`
	Rows     int64     `json:"rows"`
	Wire     uint64    `json:"wire"`
	Last     time.Time `json:"last"`
}

// StoredExporters lists the devices that have flow records kept, most
// traffic first.
func (s *Store) StoredExporters() ([]StoredExporter, error) {
	src := s.Source(time.Time{}, time.Now().Add(24*time.Hour))
	rows, err := s.DB.Query(`SELECT exporter, count(*), CAST(CASE WHEN sum(wire) > 18446744073709551615::HUGEINT THEN 18446744073709551615::HUGEINT ELSE sum(wire) END AS UBIGINT), max(ts) FROM ` + src + ` GROUP BY exporter ORDER BY 3 DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredExporter
	for rows.Next() {
		var e StoredExporter
		if err := rows.Scan(&e.Exporter, &e.Rows, &e.Wire, &e.Last); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// A flow two devices reported is stored twice, the later copy marked as a
// duplicate and left out of the totals. When the device whose copy counted
// is deleted, the flow would be counted by nobody: one remaining copy of
// each such flow (same minute and key as the pipeline's duplicate check)
// counts instead.
const (
	dupKey     = "ts, src, dst, sport, dport, proto"
	promoteDup = "CASE WHEN dup AND NOT bool_or(NOT dup) OVER k AND row_number() OVER (PARTITION BY " + dupKey + " ORDER BY exporter) = 1 THEN false ELSE dup END"
)
