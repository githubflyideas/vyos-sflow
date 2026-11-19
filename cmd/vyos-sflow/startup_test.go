package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionDefault(t *testing.T) {
	if version != "1.2.22" {
		t.Fatalf("default version = %q, want 1.2.22", version)
	}
}

func TestStartupSecurityNotice(t *testing.T) {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	for _, scheme := range []string{"http", "https"} {
		buf.Reset()
		logStartup(scheme, "0.0.0.0:8066", "0.0.0.0:6343", "/srv/flows")
		for _, text := range []string{scheme + "://0.0.0.0:8066", "sFlow UDP 0.0.0.0:6343", "Security notice:", "private/internal interface", "firewall", "TLS is optional", "No routes or firewall rules are changed"} {
			if !strings.Contains(buf.String(), text) {
				t.Errorf("missing %q in startup log: %s", text, buf.String())
			}
		}
		if scheme == "http" && !strings.Contains(buf.String(), "HTTP traffic is unencrypted") {
			t.Error("missing plaintext warning")
		}
		if scheme == "https" && strings.Contains(buf.String(), "HTTP traffic is unencrypted") {
			t.Error("incorrect plaintext warning for TLS")
		}
	}
}

func TestStartupLoopbackHint(t *testing.T) {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	for addr, want := range map[string]bool{"127.0.0.1:8066": true, "[::1]:8066": true, "localhost:8066": true, "0.0.0.0:8066": false, "[::]:8066": false, "192.168.1.2:8066": false} {
		buf.Reset()
		logStartup("http", addr, "0.0.0.0:6343", "/srv/flows")
		if got := strings.Contains(buf.String(), "reachable from this machine only"); got != want {
			t.Errorf("%s: loopback hint=%v want %v: %s", addr, got, want, buf.String())
		}
	}
}

// The local token is written into a new file only this user can read, even
// where an earlier run left a readable one or a link in its place.
func TestTokenFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".tui-token")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(p, "new-token"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v, want 0600", fi.Mode().Perm())
	}
	elsewhere := filepath.Join(dir, "elsewhere")
	os.Remove(p)
	if err := os.Symlink(elsewhere, p); err != nil {
		t.Skip(err)
	}
	if err := writePrivate(p, "new-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(elsewhere); err == nil {
		t.Error("the token was written through a link left in its place")
	}
}
