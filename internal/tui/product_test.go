package tui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductBrand(t *testing.T) {
	a := newApp(Options{Lang: "en"})
	frame := strings.Join(a.frame(), "\n")
	// Strip terminal style sequences before checking the displayed product name.
	if !strings.Contains(frame, "vyos-sflow") {
		t.Fatal("terminal header does not identify vyos-sflow")
	}
}

func TestLocalTokenRequest(t *testing.T) {
	const token = "local-service-test-token"
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if r.URL.Path != "/api/status" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"0.1.0"}`))
	}))
	defer srv.Close()
	a := newApp(Options{URL: srv.URL, Token: token, Lang: "en"})
	var got map[string]string
	if err := a.get("status", nil, &got); err != nil {
		t.Fatal(err)
	}
	if !reached || got["version"] != "0.1.0" {
		t.Fatal("service status not decoded")
	}
}
