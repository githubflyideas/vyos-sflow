package api

import (
	"encoding/json"
	"github.com/githubflyideas/vyos-sflow/internal/collector"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDataRoutesDoNotEnforceHeaderIdentity(t *testing.T) {
	c := collector.New(nil)
	if err := c.SetAllowedStream("192.0.2.1", 7); err != nil {
		t.Fatal(err)
	}
	s := &Server{Col: c}
	reached := false
	h := s.data(func(*Server, http.ResponseWriter, *http.Request) { reached = true })
	for _, f := range []string{`[{"f":"exporter","v":"192.0.2.2"}]`, `[{"f":"exporter","v":"192.0.2.1","neg":true}]`} {
		reached = false
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/api/topn?f="+url.QueryEscape(f), nil))
		if w.Code != 200 || !reached {
			t.Fatalf("exporter query blocked by obsolete identity gate: %d", w.Code)
		}
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/topn", nil))
	if !reached {
		t.Fatal("configured default query blocked")
	}
}
func TestDataRoutesNeedNoConfiguredIdentity(t *testing.T) {
	s := &Server{Col: collector.New(nil)}
	reached := false
	h := s.data(func(*Server, http.ResponseWriter, *http.Request) { reached = true })
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/topn", nil))
	if !reached || w.Code != http.StatusOK {
		t.Fatalf("default analytics blocked: %d", w.Code)
	}
	if status := s.Col.StreamStatus(); status.Enforced || status.Configured || status.Message != "" {
		t.Fatalf("unexpected identity requirement: %+v", status)
	}
}

func TestSourcesExposeConfiguredStream(t *testing.T) {
	c := collector.New(nil)
	if err := c.SetAllowedStream("2001:db8::1", 9); err != nil {
		t.Fatal(err)
	}
	c.RejectedStreams.Add(12)
	s := &Server{Col: c}
	w := httptest.NewRecorder()
	s.sources(w, httptest.NewRequest("GET", "/api/sources", nil))
	var got struct {
		Stream collector.StreamStatus `json:"stream"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Stream.Agent != "2001:db8::1" || got.Stream.SubAgent != 9 || got.Stream.Rejected != 12 || !got.Stream.Configured {
		t.Fatalf("diagnostics missing: %s", w.Body)
	}
}
