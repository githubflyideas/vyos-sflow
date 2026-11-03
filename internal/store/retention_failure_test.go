package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExistingEmptyDatabaseStillRequiresConsent(t *testing.T) {
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
	db.Close()
	opt := Options{Dir: dir, Threads: 1, EnforceSingleStream: true, StreamAgent: "192.0.2.1"}
	if s, err := Open(opt); err == nil {
		s.Close()
		t.Fatal("existing unmarked empty schema auto-authorized")
	}
	opt.AcceptRetentionChange = true
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	opt.AcceptRetentionChange = false
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestDiagnosticOnlyDatabaseBindsOnFirstConfiguredStream(t *testing.T) {
	opt := Options{Dir: t.TempDir(), Threads: 1, EnforceSingleStream: true}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta(streamAgentKey) != "" {
		t.Fatal("empty agent bound")
	}
	s.Close()
	opt.StreamAgent = "192.0.2.1"
	opt.StreamSubAgent = 4294967295
	s, err = Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta(streamSubAgentKey) != "4294967295" {
		t.Fatal("subagent identity truncated")
	}
	s.Close()
	opt.StreamAgent = "192.0.2.2"
	if s, err = Open(opt); err == nil {
		s.Close()
		t.Fatal("agent change accepted")
	}
}

func TestRetentionMissingMixedFileReportsErrorAndDoesNotMarkMigration(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	opt := Options{Dir: t.TempDir(), Threads: 1, Now: func() time.Time { return now }}
	s, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(opt.Dir, "raw", "missing.parquet")
	if _, err = s.DB.Exec("INSERT INTO segments VALUES (?,?,?,2)", path, now.Add(-49*time.Hour), now.Add(-47*time.Hour)); err != nil {
		t.Fatal(err)
	}
	before := s.LastRetentionSuccess()
	if err = s.Retain(now.Add(time.Second)); err == nil {
		t.Fatal("missing mixed file silently skipped")
	}
	if !strings.Contains(s.LastRetentionError(), "trim") || !s.LastRetentionSuccess().Equal(before) {
		t.Fatal("failure diagnostics incorrect")
	}
	if _, err = s.DB.Exec("DELETE FROM meta WHERE k=?", retentionPolicyKey); err != nil {
		t.Fatal(err)
	}
	s.Close()
	opt.AcceptRetentionChange = true
	if s, err = Open(opt); err == nil {
		s.Close()
		t.Fatal("migration succeeded despite missing flow file")
	}
	db, err := sql.Open("duckdb", filepath.Join(opt.Dir, "traffic66.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow("SELECT count(*) FROM meta WHERE k=?", retentionPolicyKey).Scan(&n); err != nil || n != 0 {
		t.Fatalf("failed migration marked: %d %v", n, err)
	}
}

func TestRetentionUnlinkFailureIsRetryable(t *testing.T) {
	now := time.Now().UTC()
	s, err := Open(Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A nonempty directory is an unlink failure on Linux and Windows alike.
	path := filepath.Join(s.opt.Dir, "raw", "blocked.parquet")
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "block")
	if err = os.WriteFile(child, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO segments VALUES (?,?,?,1)", path, now.Add(-72*time.Hour), now.Add(-71*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.Retain(now); err == nil {
		t.Fatal("unlink error ignored")
	}
	var n int
	if err = s.DB.QueryRow("SELECT count(*) FROM segments").Scan(&n); err != nil || n != 1 {
		t.Fatal("failed unlink discarded catalog")
	}
	if err = os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err = s.Retain(now); err != nil {
		t.Fatal(err)
	}
	if s.LastRetentionError() != "" {
		t.Fatal("successful retry did not clear error")
	}
}

func TestRetentionCheckpointFailureAndRetry(t *testing.T) {
	now := time.Now().UTC()
	s, err := Open(Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Inject only the CHECKPOINT I/O failure; all reads/deletes/transactions
	// still execute on the real DuckDB connector. Active transactions no
	// longer reliably make CHECKPOINT fail in modern DuckDB.
	original := s.DB
	faultDB := sql.OpenDB(checkpointFault{base: s.conn})
	s.DB = faultDB
	err = s.Retain(now)
	s.DB = original
	faultDB.Close()
	if err == nil || !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("want checkpoint failure, got %v", err)
	}
	if s.LastRetentionError() == "" {
		t.Fatal("checkpoint failure missing from diagnostics")
	}
	if err = s.Retain(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.LastRetentionError() != "" {
		t.Fatal("checkpoint retry did not clear error")
	}
}
