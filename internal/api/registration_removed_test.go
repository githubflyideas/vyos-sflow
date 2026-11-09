package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/githubflyideas/vyos-sflow/internal/collector"
	"github.com/githubflyideas/vyos-sflow/internal/dnsres"
	"github.com/githubflyideas/vyos-sflow/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow/internal/store"
)

func TestAPIWithoutRegistration(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(store.Options{Dir: dir, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{Store: st, Pipe: &pipeline.Pipeline{}, Col: collector.New(nil), DNS: dnsres.New(dnsres.Options{}), Users: map[string]string{"admin": "local-password"}, Static: fstest.MapFS{}, DataDir: dir}
	h := s.Handler()
	for _, path := range []string{"/api/status", "/api/overview?range=1h", "/api/topn?range=1h&dim=host", "/api/series?range=1h&by=app"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous: %d %s", w.Code, w.Body)
			}
			r.SetBasicAuth("admin", "local-password")
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("local account: %d %s", w.Code, w.Body)
			}
			if got := w.Header().Get("X-Traffic66-Evaluation"); got != "" {
				t.Errorf("evaluation header: %q", got)
			}
			var out map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"license", "notice"} {
				if _, ok := out[field]; ok {
					t.Errorf("unexpected %s field", field)
				}
			}
		})
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		r := httptest.NewRequest(method, "/api/license", strings.NewReader(`{"text":"obsolete activation"}`))
		r.SetBasicAuth("admin", "local-password")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("obsolete activation %s: %d %s", method, w.Code, w.Body)
		}
	}
}
