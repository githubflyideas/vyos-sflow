package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signIn gives r a session of user, as /api/login gives one; for tests
// about something other than signing in. Call it after Handler, which
// starts the session table afresh.
func signIn(s *Server, r *http.Request, user string) {
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]session{}
	}
	tok := "test-session-" + user
	now := time.Now()
	s.sessions[tok] = session{user: user, exp: now.Add(time.Hour), created: now}
	s.mu.Unlock()
	r.AddCookie(&http.Cookie{Name: cookieName, Value: tok})
}

// login signs in through h's /api/login and returns the session cookie,
// or nil when the sign-in is refused.
func login(t *testing.T, h http.Handler, user, password string) *http.Cookie {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"user":"`+user+`","password":"`+password+`"}`))
	r.RemoteAddr = "192.0.2.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c
		}
	}
	return nil
}
