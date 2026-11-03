package api

import (
	"encoding/json"
	"github.com/githubflyideas/vyos-sflow-new/internal/auth"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupHandlerRejectsLegacyAndUnconfirmedRequests(t *testing.T) {
	s := &Server{} // rejection must happen before touching storage
	for _, body := range []string{"", `{}`, `{"days":0}`, `{"all":true}`, `{"all":true,"confirm":false}`, `{"all":true,"confirm":true,"before":0}`} {
		r := httptest.NewRequest("POST", "/api/cleanup", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.postCleanup(w, r)
		if w.Code != 400 {
			t.Errorf("body %s: %d", body, w.Code)
		}
	}
	for _, method := range []string{"GET", "POST"} {
		r := httptest.NewRequest(method, "/api/cleanup?before=1", strings.NewReader(`{"all":true,"confirm":true}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		if method == "GET" {
			s.getCleanup(w, r)
		} else {
			s.postCleanup(w, r)
		}
		if w.Code != 400 {
			t.Errorf("legacy query %s: %d", method, w.Code)
		}
	}
}
func TestCleanupRouteRequiresAdmin(t *testing.T) {
	fc := auth.NewFileChecker("", map[string]string{"admin": "admin-secret", "alice": "alice-secret"})
	s := &Server{Accounts: fc, Check: fc.Check, Exists: fc.Exists}
	h := s.Handler()
	r := httptest.NewRequest("POST", "/api/cleanup", strings.NewReader(`{"all":true,"confirm":true}`))
	r.SetBasicAuth("alice", "alice-secret")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d %s", w.Code, w.Body)
	}
}
func TestCleanupHandlerClearsAllFlowHistoryPreservesIdentity(t *testing.T) {
	dir := t.TempDir()
	opt := store.Options{Dir: dir, Threads: 1, EnforceSingleStream: true, StreamAgent: "192.0.2.1", StreamSubAgent: 7}
	st, err := store.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if st != nil {
			st.Close()
		}
	}()
	p := pipeline.New(pipeline.Config{}, st)
	if _, err = st.DB.Exec("CREATE TABLE legacy_business (value INTEGER); INSERT INTO legacy_business VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "untouched-config.json")
	if err = os.WriteFile(path, []byte("existing settings"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	rec := flow.Record{Start: now.Add(-time.Hour), End: now.Add(-time.Hour), Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("10.0.0.2"), Exporter: netip.MustParseAddr("192.0.2.1"), Bytes: 100, Packets: 1, Mult: 10, Proto: 6, SrcPort: 50000, DstPort: 443, SamplingKnown: true, Source: flow.SrcSFlow}
	p.Ingest([]flow.Record{rec})
	p.FlushRows()
	p.FlushRollups()
	if err = st.Seal(now); err != nil {
		t.Fatal(err)
	}
	rec.Start, rec.End = now, now
	p.Ingest([]flow.Record{rec})
	p.Submit([]flow.Record{rec})
	if err := auth.Set(dir, "admin", "admin-secret"); err != nil {
		t.Fatal(err)
	}
	fc := auth.NewFileChecker(dir, nil)
	s := &Server{Store: st, Pipe: p, Accounts: fc, Check: fc.Check, Exists: fc.Exists}
	h := s.Handler()
	r := httptest.NewRequest("GET", "/api/cleanup", nil)
	r.SetBasicAuth("admin", "admin-secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var preview map[string]json.RawMessage
	if err = json.Unmarshal(w.Body.Bytes(), &preview); err != nil || w.Code != 200 {
		t.Fatalf("preview: %d %s %v", w.Code, w.Body, err)
	}
	if _, old := preview["options"]; old || string(preview["all"]) != "true" || preview["estimate"] == nil {
		t.Fatalf("not all-only: %s", w.Body)
	}
	r = httptest.NewRequest("POST", "/api/cleanup", strings.NewReader(`{"all":true,"confirm":true}`))
	r.SetBasicAuth("admin", "admin-secret")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("cleanup: %d %s", w.Code, w.Body)
	}
	p.FlushRows()
	p.FlushRollups()
	for _, table := range []string{"hot", "segments", "r_ts", "r_host", "r_dim"} {
		var n int
		if err = st.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s resurrected: %d %v", table, n, err)
		}
	}
	var sentinel int
	if err = st.DB.QueryRow("SELECT value FROM legacy_business").Scan(&sentinel); err != nil || sentinel != 42 {
		t.Fatalf("legacy changed: %d %v", sentinel, err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "existing settings" {
		t.Fatalf("config changed: %s %v", b, err)
	}
	if !fc.Check("admin", "admin-secret") {
		t.Fatal("account changed")
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	st = nil
	// A full cleanup must not permit changing the directory's stream identity.
	opt.StreamSubAgent = 8
	if changed, err := store.Open(opt); err == nil {
		changed.Close()
		t.Fatal("cleanup erased stream binding")
	}
	opt.StreamSubAgent = 7
	reopened, err := store.Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	st = reopened
	var remaining int
	if err = st.DB.QueryRow("SELECT count(*) FROM hot").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("history reappeared after restart: %d %v", remaining, err)
	}
	if !auth.NewFileChecker(dir, nil).Check("admin", "admin-secret") {
		t.Fatal("persisted account changed")
	}
}
