package pipeline

import (
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func instantRecord(at time.Time) flow.Record {
	return flow.Record{Start: at, End: at, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("192.0.2.2"), SrcPort: 50000, DstPort: 443, Proto: 6, Bytes: 100, Packets: 1, L2: true, Mult: 1, Source: flow.SrcSFlow, Exporter: netip.MustParseAddr("192.0.2.1"), InIf: 1}
}

func TestInstantRawTimeAndMinuteAggregates(t *testing.T) {
	p := New(Config{}, nil)
	at := time.Date(2026, 1, 1, 12, 34, 45, 123456000, time.UTC)
	first := instantRecord(at)
	same := instantRecord(at.Add(time.Second))
	dup := instantRecord(at.Add(2 * time.Second))
	dup.InIf = 2 // another observation point, same stream and minute
	next := instantRecord(at.Truncate(time.Minute).Add(time.Minute))
	next.InIf = 2
	p.Ingest([]flow.Record{first, same, dup, next})
	if len(p.batch) != 4 {
		t.Fatalf("rows: %d", len(p.batch))
	}
	for i, r := range []flow.Record{first, same, dup, next} {
		if !p.batch[i].TS.Equal(r.End) {
			t.Errorf("raw[%d] = %v, want %v", i, p.batch[i].TS, r.End)
		}
		if p.batch[i].Dup != (i == 2) {
			t.Errorf("raw[%d] duplicate = %v", i, p.batch[i].Dup)
		}
	}
	if p.DupRows.Load() != 1 || len(p.dedup) != 2 || len(p.rTS) != 2 {
		t.Fatalf("dedup/rollup counts: %d/%d/%d", p.DupRows.Load(), len(p.dedup), len(p.rTS))
	}
	for k, c := range p.rTS {
		want := uint64(100)
		if k.TS.Equal(at.Truncate(time.Minute)) {
			want = 200
		}
		if !k.TS.Equal(k.TS.Truncate(time.Minute)) || c.Wire != want {
			t.Errorf("minute rollup: %+v %+v", k, c)
		}
	}
	for k, c := range p.rHost {
		if !k.TS.Equal(at.Truncate(time.Hour)) || c.Wire != 300 {
			t.Errorf("host rollup: %+v %+v", k, c)
		}
	}
	for k, c := range p.rDim {
		if !k.TS.Equal(at.Truncate(time.Hour)) || c.Wire != 300 {
			t.Errorf("dimension rollup: %+v %+v", k, c)
		}
	}
}

// Exercise real pipeline -> DuckDB/Parquet -> exact retention, not hand-made rows.
func TestInstantRetentionCutoff(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		name := "hot"
		if sealed {
			name = "parquet"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 1, 3, 12, 34, 30, 0, time.UTC)
			cutoff := now.Add(-48 * time.Hour)
			clock := cutoff // allow insertion of the about-to-expire row
			st, err := store.Open(store.Options{Dir: t.TempDir(), Threads: 1, Now: func() time.Time { return clock }})
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			p := New(Config{}, st)
			times := []time.Time{cutoff.Add(-time.Microsecond), cutoff, cutoff.Add(15*time.Second + 123456*time.Microsecond), cutoff.Add(16 * time.Second), cutoff.Truncate(time.Hour).Add(time.Hour + 15*time.Second), now.Add(-15 * time.Second), now}
			recs := make([]flow.Record, len(times))
			for i, at := range times {
				recs[i] = instantRecord(at)
			}
			recs[3].InIf = 2 // duplicate of earlier samples in the same minute
			p.Ingest(recs)
			p.FlushRows()
			p.FlushRollups()
			if p.WriteErrs.Load() != 0 {
				t.Fatal(p.LastError())
			}
			// Read precision and alignment back before retention deletes partial buckets.
			rows, err := st.DB.Query("SELECT ts FROM hot ORDER BY ts")
			if err != nil {
				t.Fatal(err)
			}
			var stored []time.Time
			for rows.Next() {
				var at time.Time
				if err := rows.Scan(&at); err != nil {
					t.Fatal(err)
				}
				stored = append(stored, at)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			if len(stored) != len(times) {
				t.Fatalf("stored %d rows, want %d", len(stored), len(times))
			}
			for i := range times {
				if !stored[i].Equal(times[i]) {
					t.Errorf("stored timestamp %v, want %v", stored[i], times[i])
				}
			}
			for _, table := range []string{"r_ts", "r_host", "r_dim"} {
				unit := "hour"
				if table == "r_ts" {
					unit = "minute"
				}
				var n int
				if err := st.DB.QueryRow("SELECT count(*) FROM " + table + " WHERE ts <> date_trunc('" + unit + "', ts)").Scan(&n); err != nil || n != 0 {
					t.Fatalf("unaligned %s: %d %v", table, n, err)
				}
			}
			if sealed {
				if err := st.Seal(now.Add(2 * time.Hour)); err != nil {
					t.Fatal(err)
				}
				if st.Usage().Segments == 0 {
					t.Fatal("fixture was not sealed")
				}
			}
			clock = now
			if err := st.Retain(now); err != nil {
				t.Fatal(err)
			}
			var n int
			var earliest time.Time
			source := st.Source(cutoff.Add(-time.Hour), now.Add(time.Hour))
			if err := st.DB.QueryRow("SELECT count(*), min(ts) FROM "+source).Scan(&n, &earliest); err != nil || n != 6 || !earliest.Equal(cutoff) {
				t.Fatalf("retained raw: count=%d earliest=%v err=%v", n, earliest, err)
			}
			checks := []struct {
				name string
				q    store.Query
				wire uint64
			}{
				{"exact48h", store.Query{From: cutoff, To: now}, 400},
				{"same-minute-tail", store.Query{From: cutoff.Add(time.Second), To: cutoff.Add(time.Minute)}, 100},
				{"whole-hour-rollups", store.Query{From: cutoff.Truncate(time.Hour).Add(time.Hour), To: now.Truncate(time.Hour)}, 100},
			}
			for _, check := range checks {
				for _, dim := range []string{"app", "proto", "host"} {
					top, err := st.TopN(check.q, dim, 10)
					wantRows := 1
					if dim == "host" {
						wantRows = 2
					}
					if err != nil || len(top) != wantRows {
						t.Fatalf("%s %s top: %+v %v", check.name, dim, top, err)
					}
					for _, row := range top {
						if row.Wire != check.wire {
							t.Errorf("%s %s top wire=%d want=%d", check.name, dim, row.Wire, check.wire)
						}
					}
					se, err := st.SeriesBy(check.q, dim, "wire", 10)
					if err != nil {
						t.Fatal(err)
					}
					if len(se.Totals) != wantRows {
						t.Fatalf("%s %s series: %+v", check.name, dim, se)
					}
					for _, total := range se.Totals {
						if total != float64(check.wire) {
							t.Errorf("%s %s series total=%v want=%d", check.name, dim, total, check.wire)
						}
					}
					if check.name == "exact48h" {
						for _, values := range se.Values {
							if len(values) == 0 || values[len(values)-1] <= 0 {
								t.Errorf("%s missing final partial bucket", dim)
							}
						}
					}
				}
			}
			// Clear all must still clear exact timestamps, buffered rows and aggregates.
			p.Ingest([]flow.Record{instantRecord(now.Add(time.Second))})
			if _, err := p.Purge(time.Time{}); err != nil {
				t.Fatal(err)
			}
			p.FlushRows()
			p.FlushRollups()
			if p.WriteErrs.Load() != 0 {
				t.Fatal(p.LastError())
			}
			for _, table := range []string{"hot", "segments", "r_ts", "r_host", "r_dim"} {
				if err := st.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
					t.Fatalf("clear-all %s: %d %v", table, n, err)
				}
			}
		})
	}
}
