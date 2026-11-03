package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRollingRetentionMixedSegmentAndRestart(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 34, 56, 0, time.UTC)
	opt := Options{Dir: t.TempDir(), Threads: 1, Now: func() time.Time { return now }}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	cut := now.Add(-48 * time.Hour)
	// Seed while every fixture is inside the window, then advance the clock.
	now = now.Add(-time.Hour)
	rows := []Row{flowRow(cut.Add(-time.Microsecond), 10), flowRow(cut, 20), flowRow(cut.Add(time.Second), 30)}
	if err = s.AppendRows(rows); err != nil {
		t.Fatal(err)
	}
	ts := map[TSKey]*Counters{}
	host := map[HostKey]*Counters{}
	dim := map[DimKey]*Counters{}
	for _, r := range rows {
		c := &Counters{Bytes: r.Bytes, Wire: r.Wire}
		ts[TSKey{r.TS, "HTTPS"}] = c
		host[HostKey{TS: r.TS, IP: r.Src}] = c
		dim[DimKey{r.TS, "proto", "6"}] = c
	}
	if err = s.AppendRollups(ts, host, dim); err != nil {
		t.Fatal(err)
	}
	if err = s.Seal(now); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendRows(rows); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if err = s.Retain(now); err != nil {
		t.Fatal(err)
	}
	assertRetentionRows(t, s, cut)
	// Buffered old data must not resurrect after a successful cleanup.
	if err = s.AppendRows(rows[:1]); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendRollups(map[TSKey]*Counters{{TS: rows[0].TS}: {Bytes: 999}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	assertRetentionRows(t, s, cut)
	if s.LastRetentionError() != "" || !s.LastRetentionSuccess().Equal(now) {
		t.Fatal("retention diagnostics not updated")
	}
	s.Close()
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertRetentionRows(t, s, cut)
}

func assertRetentionRows(t *testing.T, s *Store, cut time.Time) {
	t.Helper()
	for _, table := range []string{"hot", "r_ts", "r_host", "r_dim"} {
		var n int
		var total uint64
		if err := s.DB.QueryRow("SELECT count(*),coalesce(sum(wire),0) FROM "+table).Scan(&n, &total); err != nil || n != 2 || total != 50 {
			t.Fatalf("%s: rows=%d wire=%d err=%v", table, n, total, err)
		}
	}
	var path string
	var n int
	var mn time.Time
	if err := s.DB.QueryRow("SELECT path,rows,min_ts FROM segments").Scan(&path, &n, &mn); err != nil || n != 2 || !mn.Equal(cut) {
		t.Fatalf("segment metadata %d %v %v", n, mn, err)
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM read_parquet('"+sqlPath(path)+"') WHERE ts < ?", cut).Scan(&n); err != nil || n != 0 {
		t.Fatalf("expired parquet head: %d %v", n, err)
	}
}

func TestRetentionMigrationRequiresConsentBeforeAnyWrite(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := sql.Open("duckdb", filepath.Join(dir, "traffic66.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range schema {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO hot (ts,wire) VALUES (?,99)", now.Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE accounts(sentinel VARCHAR); INSERT INTO accounts VALUES ('keep')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	opt := Options{Dir: dir, Threads: 1, Now: func() time.Time { return now }}
	if s, err := Open(opt); err == nil {
		s.Close()
		t.Fatal("legacy database silently accepted")
	} else if !strings.Contains(err.Error(), "-accept-48h-retention") {
		t.Fatal(err)
	}
	db, err = sql.Open("duckdb", filepath.Join(dir, "traffic66.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM hot").Scan(&n)
	if n != 1 {
		t.Fatal("unauthorized deletion")
	}
	db.Close()
	opt.AcceptRetentionChange = true
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow("SELECT count(*) FROM hot").Scan(&n)
	if n != 0 {
		t.Fatal("consented startup did not clean")
	}
	var sentinel string
	s.DB.QueryRow("SELECT sentinel FROM accounts").Scan(&sentinel)
	if sentinel != "keep" {
		t.Fatal("accounts modified")
	}
	s.Close()
	opt.AcceptRetentionChange = false
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestSingleStreamBindingRefusesIdentityChange(t *testing.T) {
	opt := Options{Dir: t.TempDir(), Threads: 1, EnforceSingleStream: true, StreamAgent: "192.0.2.1", StreamSubAgent: 7}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	opt.StreamSubAgent = 8
	if s, err = Open(opt); err == nil {
		s.Close()
		t.Fatal("changed subagent accepted")
	}
	opt.StreamSubAgent = 7
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestUnboundHistoryCannotBeClaimedBySingleStream(t *testing.T) {
	opt := Options{Dir: t.TempDir(), Threads: 1}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AppendRows([]Row{flowRow(time.Now(), 100)}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	opt.EnforceSingleStream = true
	opt.StreamAgent = "192.0.2.1"
	opt.AcceptRetentionChange = true
	if s, err = Open(opt); err == nil {
		s.Close()
		t.Fatal("unidentified history claimed by a stream")
	}
	opt.EnforceSingleStream = false
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	s.DB.QueryRow("SELECT count(*) FROM hot").Scan(&n)
	if n != 1 {
		t.Fatal("history deleted")
	}
}
