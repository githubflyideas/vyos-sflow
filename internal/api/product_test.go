package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestProductHasNoOfflineRoutes(t *testing.T) {
	b, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"/api/sandbox", "GET /auto", "internal/sandbox", "internal/capture", "internal/pcapfile"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("product route table retains %q", banned)
		}
	}
}

func TestDataRejectsOfflineSelection(t *testing.T) {
	s := &Server{}
	called := false
	h := s.data(func(_ *Server, w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) })
	for _, ds := range []string{"sb", "unknown"} {
		called = false
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest("GET", "/api/overview?ds="+ds, nil))
		if w.Code != 400 || called {
			t.Errorf("ds=%s: status=%d called=%v", ds, w.Code, called)
		}
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", "/api/overview", nil))
	if w.Code != 200 || !called {
		t.Fatal("live query was not served")
	}
}
