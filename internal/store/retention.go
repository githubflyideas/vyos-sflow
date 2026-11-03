package store

import (
	"fmt"
	"os"
	"strconv"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

const retentionPolicyKey = "flow_retention_policy"
const streamAgentKey = "flow_stream_agent"
const streamSubAgentKey = "flow_stream_subagent"

func (s *Store) policy() string {
	return fmt.Sprintf("rolling-v1:raw=%d:rollup=%d", s.opt.RawDays, s.opt.RollupDays)
}

// authorizeOpen performs only reads: neither schema migrations nor retention may
// touch an existing database until BOTH the history and stream gates succeed.
func (s *Store) authorizeOpen(existed bool) error {
	rows, err := s.DB.Query("SELECT table_name FROM information_schema.tables WHERE table_schema='main'")
	if err != nil {
		return err
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	meta := map[string]string{}
	if tables["meta"] {
		rows, err = s.DB.Query("SELECT k,v FROM meta WHERE k IN (?,?,?)", retentionPolicyKey, streamAgentKey, streamSubAgentKey)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k, v string
			if err = rows.Scan(&k, &v); err != nil {
				rows.Close()
				return err
			}
			meta[k] = v
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if s.opt.EnforceSingleStream {
		if agent, ok := meta[streamAgentKey]; ok {
			if agent != s.opt.StreamAgent || meta[streamSubAgentKey] != strconv.FormatUint(uint64(s.opt.StreamSubAgent), 10) {
				return fmt.Errorf("single-stream identity does not match this database; use a new data directory for a different agent or subagent (existing history was not changed)")
			}
		} else {
			for _, table := range []string{"hot", "segments", "r_ts", "r_host", "r_dim"} {
				if !tables[table] {
					continue
				}
				var populated bool
				if err = s.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM " + table + " LIMIT 1)").Scan(&populated); err != nil {
					return err
				}
				if populated {
					return fmt.Errorf("existing flow/aggregate history has no verified single-stream identity; use a new data directory, or back up and explicitly clear flow history first; -accept-48h-retention cannot claim or erase unidentified stream history")
				}
			}
		}
	}
	if existed && meta[retentionPolicyKey] != s.policy() && !s.opt.AcceptRetentionChange {
		return fmt.Errorf("existing database has not accepted the rolling 48-hour flow retention policy; restart with -accept-48h-retention to permanently delete flow records and flow aggregates older than 48 hours (not accounts, configuration, or unrelated legacy business data); back up the data directory first")
	}
	return nil
}

// recordPolicy is called only after startup retention AND its checkpoint succeed.
func (s *Store) recordPolicy() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	put := func(k, v string) error {
		_, err := tx.Exec("INSERT INTO meta VALUES (?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v", k, v)
		return err
	}
	if err = put(retentionPolicyKey, s.policy()); err != nil {
		return err
	}
	if s.opt.EnforceSingleStream && s.opt.StreamAgent != "" {
		if err = put(streamAgentKey, s.opt.StreamAgent); err != nil {
			return err
		}
		if err = put(streamSubAgentKey, strconv.FormatUint(uint64(s.opt.StreamSubAgent), 10)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LastRetentionError is empty after the most recent successful cleanup.
func (s *Store) LastRetentionError() string {
	s.retentionMu.RLock()
	defer s.retentionMu.RUnlock()
	return s.retentionErr
}
func (s *Store) LastRetentionSuccess() time.Time {
	s.retentionMu.RLock()
	defer s.retentionMu.RUnlock()
	return s.retentionSuccess
}

// Retain uses exact UTC instants, preserves the inclusive cutoff, and rewrites
// mixed Parquet files. A failed pass is retryable and never reported as success.
func (s *Store) Retain(now time.Time) (err error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	defer func() {
		s.retentionMu.Lock()
		defer s.retentionMu.Unlock()
		if err != nil {
			s.retentionErr = err.Error()
		} else {
			s.retentionErr = ""
			s.retentionSuccess = now.UTC()
		}
	}()
	rawCut := now.UTC().Add(-time.Duration(s.opt.RawDays) * 24 * time.Hour)
	rollCut := now.UTC().Add(-time.Duration(s.opt.RollupDays) * 24 * time.Hour)
	for _, a := range []*duckdb.Appender{s.appHot, s.appTS, s.appHost, s.appDim} {
		if err = a.Flush(); err != nil {
			return fmt.Errorf("retain: flush: %w", err)
		}
	}
	// Read the catalog, not the filtered cache: a prior failed unlink/catalog
	// operation must remain visible to the next cleanup attempt.
	rows, err := s.DB.Query("SELECT path,min_ts,max_ts FROM segments WHERE min_ts < ?", rawCut)
	if err != nil {
		return err
	}
	var segments []segment
	for rows.Next() {
		var g segment
		if err = rows.Scan(&g.path, &g.min, &g.max); err != nil {
			rows.Close()
			return err
		}
		segments = append(segments, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// If a later segment fails, refresh paths already removed by this pass.
	defer s.loadSegments()
	for _, g := range segments {
		if g.max.Before(rawCut) {
			if err = os.Remove(g.path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("retain: remove %s: %w", g.path, err)
			}
			if _, err = s.DB.Exec("DELETE FROM segments WHERE path=?", g.path); err != nil {
				return fmt.Errorf("retain: segment catalog: %w", err)
			}
		} else {
			if _, err = s.trimSegment(g, rawCut); err != nil {
				return fmt.Errorf("retain: trim %s: %w", g.path, err)
			}
		}
	}
	if err = s.loadSegments(); err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"hot", "r_ts", "r_host", "r_dim"} {
		cut := rollCut
		if table == "hot" {
			cut = rawCut
		}
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE ts < ?", cut); err != nil {
			return fmt.Errorf("retain %s: %w", table, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Always checkpoint, including a retry after deletes committed but the prior
	// checkpoint failed. Logical deletion does not promise disk-file shrinking.
	if _, err = s.DB.Exec("CHECKPOINT"); err != nil {
		return fmt.Errorf("retain: checkpoint: %w", err)
	}
	s.dsMu.Lock()
	s.dsTS = time.Time{}
	s.dsAt = time.Time{}
	s.dsMu.Unlock()
	return nil
}
