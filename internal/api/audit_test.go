package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/auth"
	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"github.com/githubflyideas/vyos-sflow/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow/internal/store"
)

// captureLog collects what the package logs during a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(flags) })
	return &buf
}

// Signing in and out, failed and refused sign-ins, and every change to
// the accounts leave a line saying who did it and from where; no password
// ever does, and a name chosen by the client cannot add a line of its own.
func TestAuditTrailOfAccounts(t *testing.T) {
	logged := captureLog(t)
	dir := t.TempDir()
	if err := auth.Set(dir, "admin", auth.DefaultPassword); err != nil {
		t.Fatal(err)
	}
	fc := auth.NewFileChecker(dir, nil)
	s := &Server{Check: fc.Check, Exists: fc.Exists, Accounts: fc, DataDir: dir, sessions: map[string]session{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	for p, h := range map[string]http.HandlerFunc{"POST /api/password": s.changePassword,
		"POST /api/users": s.putUser, "DELETE /api/users": s.deleteUser} {
		mux.Handle(p, s.auth(h))
	}
	call := func(method, path, body, cookie, ip string) (int, string) {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = ip + ":1"
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieName {
				cookie = c.Value
			}
		}
		return w.Code, cookie
	}
	const newPw, bobPw, bobPw2 = "s3cret-password-1", "bob-password-123", "bob-password-456"
	if c, _ := call("POST", "/api/login", `{"user":"nobody","password":"guess-guess-1"}`, "", "192.0.2.7"); c != 401 {
		t.Fatalf("unknown user: %d", c)
	}
	_, ck := call("POST", "/api/login", `{"user":"admin","password":"`+auth.DefaultPassword+`"}`, "", "192.0.2.1")
	call("POST", "/api/password", `{"old":"`+auth.DefaultPassword+`","new":"`+newPw+`"}`, ck, "192.0.2.1")
	call("POST", "/api/users", `{"user":"bob","password":"`+bobPw+`"}`, ck, "192.0.2.1")
	call("POST", "/api/users", `{"user":"bob","password":"`+bobPw2+`","reset":true}`, ck, "192.0.2.1")
	call("DELETE", "/api/users?user=bob", "", ck, "192.0.2.1")
	call("POST", "/api/logout", "", ck, "192.0.2.1")
	// a name that tries to start a line of its own
	call("POST", "/api/login", `{"user":"x\naudit: user deleted user=\"admin\"","password":"guess-guess-1"}`, "", "192.0.2.9")
	for range maxFails {
		call("POST", "/api/login", `{"user":"admin","password":"guess-guess-1"}`, "", "198.51.100.5")
	}
	call("POST", "/api/login", `{"user":"admin","password":"`+newPw+`"}`, "", "198.51.100.5")

	out := logged.String()
	for _, want := range []string{
		`audit: login failed user="nobody" by="" ip=192.0.2.7`,
		`audit: login user="admin" by="" ip=192.0.2.1`,
		`audit: password changed user="admin" by="admin" ip=192.0.2.1`,
		`audit: user created user="bob" by="admin" ip=192.0.2.1`,
		`audit: password reset user="bob" by="admin" ip=192.0.2.1`,
		`audit: user deleted user="bob" by="admin" ip=192.0.2.1`,
		`audit: logout user="admin" by="" ip=192.0.2.1`,
		`audit: login locked out for a minute user="admin" by="" ip=198.51.100.5`,
		`audit: login refused while locked out user="admin" by="" ip=198.51.100.5`,
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, pw := range []string{auth.DefaultPassword, newPw, bobPw, bobPw2, "guess-guess-1"} {
		if strings.Contains(out, pw) {
			t.Errorf("a password is in the log: %q", pw)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(line, "audit: ") || !strings.Contains(line, " ip=") {
			t.Errorf("a line that is not an audit entry: %q", line)
		}
	}
}

// Deleting one device's data says whose data, how much, and who did it.
func TestAuditTrailOfDeviceDeletion(t *testing.T) {
	logged := captureLog(t)
	dir := t.TempDir()
	st, err := store.Open(store.Options{Dir: dir, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := pipeline.New(pipeline.Config{}, st)
	now := time.Now().UTC().Truncate(time.Minute)
	rec := flow.Record{Start: now, End: now, Src: netip.MustParseAddr("10.0.0.1"), Dst: netip.MustParseAddr("10.0.0.2"),
		Exporter: netip.MustParseAddr("192.0.2.44"), Bytes: 100, Packets: 1, Mult: 1, Proto: 6, SrcPort: 50000, DstPort: 443,
		SamplingKnown: true, Source: flow.SrcSFlow}
	p.Ingest([]flow.Record{rec})
	p.FlushRows()
	if err := auth.Set(dir, "admin", "admin-secret-1"); err != nil {
		t.Fatal(err)
	}
	fc := auth.NewFileChecker(dir, nil)
	s := &Server{Store: st, Pipe: p, Accounts: fc, Check: fc.Check, Exists: fc.Exists}
	r := httptest.NewRequest("POST", "/api/cleanup/exporter", strings.NewReader(`{"exporter":"192.0.2.44","confirm":true}`))
	r.SetBasicAuth("admin", "admin-secret-1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if want := `audit: device flow records deleted exporter="192.0.2.44" records="`; !strings.Contains(logged.String(), want) ||
		!strings.Contains(logged.String(), `by="admin" ip=192.0.2.1`) {
		t.Errorf("missing %q in\n%s", want, logged)
	}
}
