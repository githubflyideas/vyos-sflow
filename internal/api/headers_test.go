package api

import (
	"io/fs"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/githubflyideas/vyos-sflow/internal/web"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	s := &Server{Static: web.FS(), Users: map[string]string{"admin": "secret"}}
	h := s.Handler()
	for _, path := range []string{"/", "/index.html", "/app.js", "/app.css", "/api/overview", "/api/me", "/api/nope"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		for name, want := range map[string]string{
			"Content-Security-Policy": ContentSecurityPolicy,
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
		} {
			if got := w.Header().Get(name); got != want {
				t.Errorf("%s: %s = %q, want %q", path, name, got, want)
			}
		}
	}
	if !strings.Contains(ContentSecurityPolicy, "frame-ancestors 'none'") || !strings.Contains(ContentSecurityPolicy, "script-src 'self';") {
		t.Fatal("policy must forbid framing and non-self scripts")
	}
}

func TestLogoKeepsStricterPolicy(t *testing.T) {
	s := &Server{Static: web.FS()}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/logo", nil))
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") || !strings.HasPrefix(got, "default-src 'none'") {
		t.Fatalf("logo policy = %q", got)
	}
}

// The policy forbids inline scripts and event-handler attributes; the
// embedded page must therefore load every script from a same-origin file.
func TestEmbeddedPageCompatibleWithPolicy(t *testing.T) {
	html, err := fs.ReadFile(web.FS(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	scripts := regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`).FindAllStringSubmatch(page, -1)
	if len(scripts) == 0 {
		t.Fatal("no scripts found")
	}
	for _, m := range scripts {
		src := regexp.MustCompile(`\bsrc="([^"]+)"`).FindStringSubmatch(m[1])
		if src == nil || strings.TrimSpace(m[2]) != "" {
			t.Errorf("inline script would be blocked by CSP: %q", m[0])
			continue
		}
		if strings.Contains(src[1], "//") || strings.HasPrefix(src[1], "data:") {
			t.Errorf("non-same-origin script %q", src[1])
		}
		if _, err := fs.Stat(web.FS(), strings.TrimPrefix(src[1], "/")); err != nil {
			t.Errorf("script %q is not embedded: %v", src[1], err)
		}
	}
	if m := regexp.MustCompile(`(?i)\son[a-z]+\s*=`).FindString(page); m != "" {
		t.Errorf("inline event handler attribute would be blocked by CSP: %q", m)
	}
	for _, m := range regexp.MustCompile(`(?i)<link\b[^>]*\bhref="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		if strings.Contains(m[1], "//") && !strings.HasPrefix(m[1], "data:") {
			t.Errorf("external resource %q is not allowed by CSP", m[1])
		}
	}
	for _, name := range []string{"app.js", "theme.js"} {
		b, err := fs.ReadFile(web.FS(), name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"eval(", "new Function(", "javascript:", "import("} {
			if strings.Contains(string(b), banned) {
				t.Errorf("%s uses %q, which the CSP blocks", name, banned)
			}
		}
	}
}
