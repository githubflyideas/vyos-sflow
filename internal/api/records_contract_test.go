package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func TestRecordsEffectiveWindowMetadata(t *testing.T) {
	st, err := store.Open(store.Options{Dir: t.TempDir(), Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	to := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		hours, limit, wantLimit int
		filter                  bool
	}{
		{47, 999, 500, false}, {47, 500, 500, true}, {6, 12, 12, false}, {1, 0, 100, true},
	} {
		t.Run(fmt.Sprintf("%dh_limit%d_filter%t", tc.hours, tc.limit, tc.filter), func(t *testing.T) {
			from := to.Add(-time.Duration(tc.hours) * time.Hour)
			params := url.Values{"from": {fmt.Sprint(from.UnixMilli())}, "to": {fmt.Sprint(to.UnixMilli())}, "limit": {fmt.Sprint(tc.limit)}}
			if tc.filter {
				params.Set("f", `[{"f":"ip","v":"203.0.113.9"}]`)
			}
			req := httptest.NewRequest("GET", "/api/records?"+params.Encode(), nil)
			global, err := parseQuery(req)
			if err != nil {
				t.Fatal(err)
			}
			if !global.From.Equal(from) {
				t.Fatal("global query window was clamped")
			}
			w := httptest.NewRecorder()
			s.records(w, req)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			var got struct {
				From, To time.Time
				Limit    int
				Rows     []store.Record
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			wantFrom := from
			if tc.hours > 24 {
				wantFrom = to.Add(-24 * time.Hour)
			}
			if !got.From.Equal(wantFrom) || !got.To.Equal(to) || got.Limit != tc.wantLimit {
				t.Fatalf("effective metadata = %+v, want %s..%s limit %d", got, wantFrom, to, tc.wantLimit)
			}
			if len(got.Rows) != 0 {
				t.Fatalf("empty store returned %d rows", len(got.Rows))
			}
		})
	}
}
