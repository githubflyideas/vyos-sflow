package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStoreHasOnlyBasicTables(t *testing.T) {
	s := chartStore(t)
	for _, name := range []string{"ifc", "findings", "seen", "r_ts_if", "r_dim_if"} {
		var n int
		if err := s.DB.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_name=?", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("obsolete table created: %s", name)
		}
	}
}

func TestOldBusinessDataIsNotMigratedOrDeleted(t *testing.T) {
	dir := t.TempDir()
	opt := Options{Dir: dir, Threads: 1}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"ifc", "findings", "seen", "r_ts_if", "r_dim_if"}
	for _, name := range names {
		if _, err := s.DB.Exec("CREATE TABLE " + name + " (sentinel VARCHAR)"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec("INSERT INTO " + name + " VALUES ('existing user data')"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "inventory.txt")
	if err := os.WriteFile(legacy, []byte("existing inventory"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Retain(time.Now()); err != nil {
		t.Fatal(err)
	}
	// Explicit cleanup may delete current flow data, never legacy business data.
	for _, cutoff := range []time.Time{time.Now(), {}} {
		if err := s.AppendRows([]Row{flowRow(time.Now().Add(-time.Hour), 100)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PurgeBefore(cutoff); err != nil {
			t.Fatal(err)
		}
		var remaining int
		if err := s.DB.QueryRow("SELECT count(*) FROM hot").Scan(&remaining); err != nil || remaining != 0 {
			t.Fatalf("flow cleanup failed: rows=%d err=%v", remaining, err)
		}
	}
	for _, name := range names {
		var got string
		if err := s.DB.QueryRow("SELECT sentinel FROM " + name).Scan(&got); err != nil || got != "existing user data" {
			t.Fatalf("%s altered: %q %v", name, got, err)
		}
	}
	if b, err := os.ReadFile(legacy); err != nil || string(b) != "existing inventory" {
		t.Fatalf("legacy file changed: %s %v", b, err)
	}
}

func TestBasicDimensionsAndNoAdvancedFilters(t *testing.T) {
	s := chartStore(t)
	now := time.Now().UTC().Truncate(time.Minute).Add(-time.Hour)
	row := flowRow(now, 100)
	row.Proto = 6
	if err := s.AppendRows([]Row{row}); err != nil {
		t.Fatal(err)
	}
	q := Query{From: now, To: now.Add(time.Minute)}
	for _, dim := range []string{"host", "client", "server", "app", "proto"} {
		top, err := s.TopN(q, dim, 10)
		if err != nil || len(top) == 0 {
			t.Fatalf("%s: %+v %v", dim, top, err)
		}
		if dim == "client" || dim == "server" {
			continue // ranked, not drawn
		}
		se, err := s.SeriesBy(q, dim, "wire", 8)
		if err != nil || len(se.Names) == 0 {
			t.Fatalf("%s: %+v %v", dim, se, err)
		}
	}
	for _, dim := range []string{"conv", "service", "country", "asn", "org", "threat", "segment", "dir", "exporter", "port", "vlan", "encap"} {
		if _, err := s.TopN(q, dim, 10); err == nil {
			t.Errorf("removed top dimension accepted: %s", dim)
		}
	}
	for _, field := range []string{"country", "asn", "org", "threat", "segment", "client", "server", "dir", "bytes", "vlan", "encap"} {
		qq := q
		qq.Filters = []Filter{{Field: field, Value: "x"}}
		if _, err := s.Totals(qq); err == nil {
			t.Errorf("removed filter accepted: %s", field)
		}
	}
	for _, measure := range []string{"peers", "avg"} {
		if _, err := s.TopNBy(q, "host", 10, measure); err == nil {
			t.Errorf("advanced ranking accepted: %s", measure)
		}
	}
}

func TestHistoryQueriesNeverWidenIntoOlderBuckets(t *testing.T) {
	s := chartStore(t)
	now := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	from := now.Add(-7 * 24 * time.Hour).Add(30 * time.Minute)
	older := flowRow(from.Add(-time.Minute), 9000)
	inside := flowRow(from.Add(time.Minute), 100)
	if err := s.AppendRows([]Row{older, inside}); err != nil {
		t.Fatal(err)
	}
	q := Query{From: from, To: now}
	for _, get := range []func() (*Series, error){func() (*Series, error) { return s.SeriesByApp(q, 8) }, func() (*Series, error) { return s.SeriesBy(q, "app", "wire", 8) }} {
		se, err := get()
		if err != nil {
			t.Fatal(err)
		}
		total := 0.0
		for _, values := range se.Values {
			for i, v := range values {
				total += v * bucketSecs(time.UnixMilli(se.Times[0]), i, time.Duration(se.Step)*time.Second, q.To, s.DataStart()) / 8
			}
		}
		if total < 99.99 || total > 100.01 {
			t.Fatalf("read outside requested history: %v bytes", total)
		}
	}
}
