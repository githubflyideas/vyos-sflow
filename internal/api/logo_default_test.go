package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLogoUsesOnlyEmbeddedDefault(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "logo.svg")
	if err := os.WriteFile(legacy, []byte("custom brand"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{DataDir: dir, Static: fstest.MapFS{"logo.svg": {Data: []byte(`<svg fill="#1b2432">default</svg>`)}}}
	w := httptest.NewRecorder()
	s.logo(w, httptest.NewRequest("GET", "/logo?dark=1", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "default") || strings.Contains(w.Body.String(), "custom brand") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("logo security headers missing")
	}
	if b, err := os.ReadFile(legacy); err != nil || string(b) != "custom brand" {
		t.Fatalf("legacy logo modified: %s %v", b, err)
	}
}
