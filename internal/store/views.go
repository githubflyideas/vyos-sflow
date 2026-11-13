package store

import (
	"strings"
	"time"
)

// Use summaries only for unfiltered, complete hourly windows. Never widen
// an explicit range or read an older bucket beyond the allowed query window.
func (s *Store) rollupOK(q Query, hosts bool) bool {
	return !q.IncludeDup && len(q.Filters) == 0 && q.span() > 6*time.Hour && s.summariesOK(q) && q.From.Equal(q.From.Truncate(time.Hour)) && q.To.Equal(q.To.Truncate(time.Hour))
}
func (s *Store) rollups(q Query, sqlq string) (string, []any, error) {
	for _, table := range []string{"r_ts", "r_host", "r_dim"} {
		sqlq = strings.ReplaceAll(sqlq, "{"+table+"}", table)
	}
	return sqlq, nil, nil
}
