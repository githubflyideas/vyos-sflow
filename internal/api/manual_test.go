package api

import (
	"github.com/githubflyideas/vyos-sflow/docs"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManual(t *testing.T) {
	s := &Server{}
	get := func(url string, h http.HandlerFunc, name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", url, nil)
		if name != "" {
			r.SetPathValue("name", name)
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	// This extraction ships one product-specific English manual. Unsupported
	// languages (including Japanese) must return that exact fallback resource.
	want, err := fs.ReadFile(docs.FS, "MANUAL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(want), "# vyos-sflow operator manual") {
		t.Fatal("embedded product manual missing")
	}
	for _, l := range []string{"ja", "xx", "../x", "", "en"} {
		w := get("/api/manual?lang="+l, s.manual, "")
		if w.Code != 200 || w.Body.String() != string(want) || w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
			t.Errorf("%q does not return the product manual: %d", l, w.Code)
		}
	}
	// Upstream screenshots are intentionally not shipped as screenshots of
	// this product. Missing assets and traversal attempts must remain 404.
	for _, n := range []string{"overview.png", "../MANUAL.md", "MANUAL.md", "x.png"} {
		if w := get("/", s.manualImage, n); w.Code != 404 {
			t.Errorf("%q: %d", n, w.Code)
		}
	}
}
