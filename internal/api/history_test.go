package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/store"
)

func TestHistoryWindow(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 34, 56, 789000000, time.UTC)
	for _, tc := range []struct {
		q    string
		span time.Duration
		bad  bool
	}{
		{"", time.Hour, false}, {"range=48h", 48 * time.Hour, false}, {"range=7d", 0, true}, {"range=30d", 0, true},
		{"range=", 0, true}, {"range=unknown", 0, true}, {"from=1", 0, true}, {"to=1", 0, true}, {"from=&to=", 0, true},
		{fmt.Sprintf("range=30d&from=%d&to=%d", now.Add(-time.Hour).UnixMilli(), now.UnixMilli()), 0, true},
		{fmt.Sprintf("from=%d&to=%d", now.Add(-8*24*time.Hour).UnixMilli(), now.UnixMilli()), 0, true},
		{fmt.Sprintf("from=%d&to=%d", now.Add(-8*24*time.Hour).UnixMilli(), now.Add(-48*time.Hour).UnixMilli()), 0, true},
		{fmt.Sprintf("from=%d&to=%d", now.Add(-48*time.Hour).UnixMilli(), now.UnixMilli()), 48 * time.Hour, false},
		{fmt.Sprintf("from=%d&to=%d", now.UnixMilli(), now.Add(time.Hour).UnixMilli()), 0, true},
		{fmt.Sprintf("from=%d&to=%d", now.Add(-48*time.Hour-time.Millisecond).UnixMilli(), now.Add(-time.Millisecond).UnixMilli()), 0, true},
		{fmt.Sprintf("from=%d&to=%d", now.Add(-72*time.Hour).UnixMilli(), now.Add(-24*time.Hour).UnixMilli()), 0, true},
		{fmt.Sprintf("range=7d&from=%d&to=%d", now.Add(-time.Hour).UnixMilli(), now.UnixMilli()), 0, true},
	} {
		v, _ := url.ParseQuery(tc.q)
		from, to, err := historyWindow(v, now)
		if (err != nil) != tc.bad {
			t.Errorf("%q err=%v want bad=%v", tc.q, err, tc.bad)
			continue
		}
		if err == nil && (to.Sub(from) != tc.span || from.Before(now.Add(-48*time.Hour))) {
			t.Errorf("%q range %s..%s", tc.q, from, to)
		}
	}
}

// The overview answers with what its page draws and nothing else: ranking
// every address of two days for a list no page shows made the 24- and
// 48-hour views take many seconds.
func TestOverviewOmitsWhatNoPageShows(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(store.Options{Dir: dir, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{Store: st}
	w := httptest.NewRecorder()
	s.overview(w, httptest.NewRequest("GET", "/api/overview?range=48h", nil))
	if w.Code != 200 {
		t.Fatalf("overview: %d %s", w.Code, w.Body)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"totals", "series", "now_bps", "peak_bps"} {
		if _, ok := out[k]; !ok {
			t.Errorf("overview lacks %q", k)
		}
	}
	for _, k := range []string{"top_hosts", "top_apps", "proto"} {
		if _, ok := out[k]; ok {
			t.Errorf("overview still computes %q", k)
		}
	}
	if strings.Contains(string(out["totals"]), "hosts") {
		t.Errorf("overview still counts hosts: %s", out["totals"])
	}
}
