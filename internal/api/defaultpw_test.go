package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/githubflyideas/vyos-sflow/internal/auth"
)

// newInstallation is a fresh installation: one admin, still on the
// password every installation starts with.
func newInstallation(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()
	if err := auth.Set(dir, "admin", auth.DefaultPassword); err != nil {
		t.Fatal(err)
	}
	fc := auth.NewFileChecker(dir, nil)
	s := &Server{Check: fc.Check, Exists: fc.Exists, Accounts: fc, DataDir: dir, sessions: map[string]session{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/loginhint", s.loginHint)
	mux.Handle("GET /api/me", s.auth(s.me))
	mux.Handle("POST /api/password", s.auth(s.changePassword))
	mux.Handle("GET /api/overview", s.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	return s, mux
}

// TestBasicAuthObeysTheForcedChange: the forced password change used to be
// checked only where a session carried it, so Basic auth walked past it
// and the whole API answered to admin and the starting password.
func TestBasicAuthObeysTheForcedChange(t *testing.T) {
	_, mux := newInstallation(t)
	basic := func(path, user, pass string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "192.0.2.1:1"
		r.SetBasicAuth(user, pass)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	w := basic("/api/overview", "admin", auth.DefaultPassword)
	if w.Code != http.StatusForbidden {
		t.Errorf("Basic auth with the starting password: got %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "change_password") {
		t.Errorf("expected change_password, got %s", w.Body)
	}
	// the calls needed to get out of that state still answer
	if w := basic("/api/me", "admin", auth.DefaultPassword); w.Code != http.StatusOK {
		t.Errorf("/api/me before the change: got %d, want 200", w.Code)
	}
	// and once the password is a real one, Basic auth works as before
	r := httptest.NewRequest("POST", "/api/password", strings.NewReader(`{"old":"`+auth.DefaultPassword+`","new":"s3cret-pass"}`))
	r.RemoteAddr = "192.0.2.1:1"
	r.SetBasicAuth("admin", auth.DefaultPassword)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("change over Basic auth: got %d %s", rec.Code, rec.Body)
	}
	if w := basic("/api/overview", "admin", "s3cret-pass"); w.Code != http.StatusOK {
		t.Errorf("Basic auth after the change: got %d, want 200", w.Code)
	}
}

// The machine token must not bypass onboarding.
func TestLocalTokenObeysTheForcedChange(t *testing.T) {
	s, mux := newInstallation(t)
	s.LocalTok = "test-machine-token"
	call := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call("/api/overview", s.LocalTok); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"error":"change_password"`) {
		t.Fatalf("local token before change: %d %s", w.Code, w.Body)
	}
	if w := call("/api/overview", "wrong-token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: %d %s", w.Code, w.Body)
	}
	if w := call("/api/me", s.LocalTok); w.Code != http.StatusOK {
		t.Fatalf("onboarding endpoint: %d %s", w.Code, w.Body)
	}
	// Complete the real password endpoint using the factory credentials.
	r := httptest.NewRequest("POST", "/api/password", strings.NewReader(`{"old":"`+auth.DefaultPassword+`","new":"changed-password"}`))
	r.SetBasicAuth("admin", auth.DefaultPassword)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", w.Code, w.Body)
	}
	if w := call("/api/overview", s.LocalTok); w.Code != http.StatusOK {
		t.Fatalf("local token after change: %d %s", w.Code, w.Body)
	}
}

func TestLocalTokenForcedChangeExemptions(t *testing.T) {
	for _, mode := range []string{"demo", "fixed", "no-accounts"} {
		t.Run(mode, func(t *testing.T) {
			s, mux := newInstallation(t)
			s.LocalTok = "test-machine-token"
			switch mode {
			case "demo":
				s.Demo = true
			case "fixed":
				s.Accounts = auth.NewFileChecker(s.DataDir, map[string]string{"admin": auth.DefaultPassword})
			case "no-accounts":
				s.Accounts = nil
			}
			r := httptest.NewRequest("GET", "/api/overview", nil)
			r.Header.Set("Authorization", "Bearer "+s.LocalTok)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("exempt local token: %d %s", w.Code, w.Body)
			}
		})
	}
}

// TestLoginHintKeepsThePassword: the sign-in page may say that an
// installation is still on its first password. It may not say what it is,
// to a caller that has not signed in.
func TestLoginHintKeepsThePassword(t *testing.T) {
	s, mux := newInstallation(t)
	hint := func() string {
		r := httptest.NewRequest("GET", "/api/loginhint", nil)
		r.RemoteAddr = "192.0.2.1:1"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Body.String()
	}
	got := hint()
	if !strings.Contains(got, `"default":true`) {
		t.Errorf("a fresh installation should still say so: %s", got)
	}
	if strings.Contains(got, auth.DefaultPassword) {
		t.Errorf("the hint hands out the password: %s", got)
	}
	if strings.Contains(got, `"password"`) {
		t.Errorf("the hint carries a password field: %s", got)
	}
	// the demo is signed into by strangers on purpose, and still says how
	s.Demo = true
	if got := hint(); !strings.Contains(got, auth.DefaultPassword) {
		t.Errorf("the demo should still say how to sign in: %s", got)
	}
}

// TestChangingThePasswordEndsOtherSessions: a password is changed because
// the old one is no longer trusted, so what it opened goes with it — but
// not the session doing the changing.
func TestChangingThePasswordEndsOtherSessions(t *testing.T) {
	_, mux := newInstallation(t)
	login := func() string {
		r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"admin","password":"`+auth.DefaultPassword+`"}`))
		r.RemoteAddr = "192.0.2.1:1"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				return c.Value
			}
		}
		t.Fatalf("no session: %d %s", w.Code, w.Body)
		return ""
	}
	stolen, mine := login(), login()
	call := func(method, path, body, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.1:1"
		r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call("POST", "/api/password", `{"old":"`+auth.DefaultPassword+`","new":"s3cret-pass"}`, mine); w.Code != http.StatusOK {
		t.Fatalf("change: %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/api/overview", "", mine); w.Code != http.StatusOK {
		t.Errorf("the session that changed the password: got %d, want 200", w.Code)
	}
	if w := call("GET", "/api/overview", "", stolen); w.Code != http.StatusUnauthorized {
		t.Errorf("a session opened with the old password: got %d, want 401", w.Code)
	}
}
