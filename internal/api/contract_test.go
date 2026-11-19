package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/githubflyideas/vyos-sflow/internal/auth"
)

func contractServer(t *testing.T) http.Handler {
	t.Helper()
	fc := auth.NewFileChecker("", map[string]string{"admin": "admin-secret-1"})
	s := &Server{Accounts: fc, Check: fc.Check, Exists: fc.Exists, Static: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}}
	return s.Handler()
}

func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.SetBasicAuth("admin", "admin-secret-1")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// errorOf reads an error answer and checks its shape: JSON, a known code
// and a message.
func errorOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%d answered as %q: %s", w.Code, ct, w.Body)
	}
	var e struct{ Error, Message string }
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("not JSON: %s", w.Body)
	}
	if _, ok := errorText[e.Error]; !ok || e.Message == "" {
		t.Fatalf("error %q is not in the code table, or has no message: %s", e.Error, w.Body)
	}
	return e.Error
}

// A wrong method is 405 with Allow and the JSON error, for every route
// and for its /api/v1 name; nothing runs.
func TestWrongMethodIs405WithAllow(t *testing.T) {
	h := contractServer(t)
	s := &Server{}
	s.Handler()
	for _, pat := range s.routes {
		method, path, _ := strings.Cut(pat, " ")
		path = strings.Replace(path, "{name}", "x.png", 1)
		wrong := "PUT"
		if method == "PUT" {
			wrong = "PATCH"
		}
		for _, p := range []string{path, strings.Replace(path, "/api/", "/api/v1/", 1)} {
			w := call(h, wrong, p, "")
			if w.Code != http.StatusMethodNotAllowed || !strings.Contains(w.Header().Get("Allow"), method) {
				t.Errorf("%s %s: %d, Allow %q; want 405 allowing %s", wrong, p, w.Code, w.Header().Get("Allow"), method)
				continue
			}
			if code := errorOf(t, w); code != "method_not_allowed" {
				t.Errorf("%s %s: code %q", wrong, p, code)
			}
		}
	}
}

// An unknown API path is a JSON 404, whatever the method; the web pages
// are still served.
func TestUnknownRouteIsJSON404(t *testing.T) {
	h := contractServer(t)
	for _, c := range []struct{ method, path string }{{"GET", "/api/nope"}, {"POST", "/api/nope"}, {"GET", "/api/v1/nope"}, {"DELETE", "/api/"}} {
		w := call(h, c.method, c.path, "")
		if w.Code != http.StatusNotFound || errorOf(t, w) != "not_found" {
			t.Errorf("%s %s: %d %s", c.method, c.path, w.Code, w.Body)
		}
	}
	if w := call(h, "GET", "/", ""); w.Code != 200 {
		t.Errorf("the web page: %d", w.Code)
	}
}

// /api/v1/ answers as /api/ does.
func TestVersionedPrefix(t *testing.T) {
	h := contractServer(t)
	a, b := call(h, "GET", "/api/me", ""), call(h, "GET", "/api/v1/me", "")
	if a.Code != 200 || b.Code != 200 || a.Body.String() != b.Body.String() {
		t.Errorf("/api/me %d %s, /api/v1/me %d %s", a.Code, a.Body, b.Code, b.Body)
	}
}

// A body that is not JSON is bad_json, and the decoder's own wording does
// not reach the client.
func TestBadBodyIsACode(t *testing.T) {
	h := contractServer(t)
	for _, path := range []string{"/api/login", "/api/resolve", "/api/v1/resolve"} {
		w := call(h, "POST", path, `{"ips": 7`)
		if w.Code != 400 || errorOf(t, w) != "bad_json" {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "unexpected") || strings.Contains(w.Body.String(), "json:") {
			t.Errorf("%s: decoder text reached the client: %s", path, w.Body)
		}
	}
}

// Request fields are snake_case; the capitalised names earlier clients
// sent still work.
func TestLowercaseAndOldFieldNames(t *testing.T) {
	h := contractServer(t)
	for _, body := range []string{`{"user":"admin","password":"wrong-password-1"}`, `{"User":"admin","Password":"wrong-password-1"}`} {
		w := call(h, "POST", "/api/login", body)
		if w.Code != http.StatusUnauthorized || errorOf(t, w) != "wrong_login" {
			t.Errorf("%s: %d %s", body, w.Code, w.Body)
		}
	}
}

// docs/openapi.json describes exactly the routes the server registers, and
// its error codes are exactly the code table.
func TestOpenAPIMatchesTheRoutes(t *testing.T) {
	b, err := os.ReadFile("../../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage
		Components struct {
			Schemas struct {
				Error struct {
					Properties struct {
						Error struct{ Enum []string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var documented, registered []string
	for p, ops := range doc.Paths {
		for m := range ops {
			documented = append(documented, strings.ToUpper(m)+" /api"+p)
		}
	}
	s := &Server{}
	s.Handler()
	registered = append(registered, s.routes...)
	sort.Strings(documented)
	sort.Strings(registered)
	if strings.Join(documented, "\n") != strings.Join(registered, "\n") {
		t.Errorf("docs/openapi.json and the route table differ:\ndocumented:\n%s\nregistered:\n%s", strings.Join(documented, "\n"), strings.Join(registered, "\n"))
	}
	var codes []string
	for c := range errorText {
		codes = append(codes, c)
	}
	enum := doc.Components.Schemas.Error.Properties.Error.Enum
	sort.Strings(codes)
	sort.Strings(enum)
	if strings.Join(codes, ",") != strings.Join(enum, ",") {
		t.Errorf("error codes documented %v, in the code table %v", enum, codes)
	}
}
