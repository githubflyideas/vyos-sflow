package api

import (
	"net/http"
	"net/http/httptest"

	"strings"
	"testing"

	"github.com/githubflyideas/vyos-sflow-new/internal/auth"
)

// TestAdminOnly: a signed-in user who is not the administrator is
// refused, the administrator is let through, and so is this machine's
// own token, which the TUI carries.
func TestAdminOnly(t *testing.T) {
	dir := t.TempDir()
	for _, u := range [][2]string{{"admin", "admin-pass"}, {"alice", "alice-pass"}} {
		if err := auth.Set(dir, u[0], u[1]); err != nil {
			t.Fatal(err)
		}
	}
	fc := auth.NewFileChecker(dir, nil)
	s := &Server{Check: fc.Check, Exists: fc.Exists, Accounts: fc, DataDir: dir,
		LocalTok: "local-token", sessions: map[string]session{}}
	reached := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("POST /gated", s.auth(s.adminOnly(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})))
	login := func(user, pass string) string {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"`+user+`","password":"`+pass+`"}`))
		r.RemoteAddr = "192.0.2.1:1"
		mux.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				return c.Value
			}
		}
		t.Fatalf("no session for %s: %d %s", user, w.Code, w.Body)
		return ""
	}
	post := func(cookie, bearer string) int {
		reached = false
		r := httptest.NewRequest("POST", "/gated", nil)
		r.RemoteAddr = "192.0.2.1:1"
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if code := post(login("alice", "alice-pass"), ""); code != http.StatusForbidden {
		t.Errorf("alice: got %d, want 403", code)
	}
	if reached {
		t.Error("alice reached the handler")
	}
	if code := post(login("admin", "admin-pass"), ""); code != http.StatusOK {
		t.Errorf("admin: got %d, want 200", code)
	}
	if !reached {
		t.Error("admin did not reach the handler")
	}
	if code := post("", "local-token"); code != http.StatusOK {
		t.Errorf("local token: got %d, want 200", code)
	}
	if !reached {
		t.Error("the local token did not reach the handler")
	}
	if code := post("", "wrong-token"); code != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d, want 401", code)
	}
}

// TestAdminOnlyWhenPasswordsAreFixed is the regression this gate could
// easily have been: -password keeps the accounts in memory and leaves no
// password file to edit, so admin() answers 409 "fixed". Gating on that
// would lock such an installation out of its own settings.
func TestAdminOnlyWhenPasswordsAreFixed(t *testing.T) {
	fc := auth.NewFileChecker("", map[string]string{"admin": "admin-pass"})
	if !fc.Fixed() {
		t.Fatal("expected a fixed checker")
	}
	s := &Server{Check: fc.Check, Exists: fc.Exists, Accounts: fc, sessions: map[string]session{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("POST /gated", s.auth(s.adminOnly(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"admin","password":"admin-pass"}`))
	r.RemoteAddr = "192.0.2.1:1"
	mux.ServeHTTP(w, r)
	var ck string
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			ck = c.Value
		}
	}
	if ck == "" {
		t.Fatalf("no session: %d %s", w.Code, w.Body)
	}
	r = httptest.NewRequest("POST", "/gated", nil)
	r.RemoteAddr = "192.0.2.1:1"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: ck})
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("admin with a fixed password file: got %d %s, want 200", w.Code, w.Body)
	}
}
