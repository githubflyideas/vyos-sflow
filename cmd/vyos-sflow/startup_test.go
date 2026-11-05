package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestReleaseVersionDefault(t *testing.T) {
	if version != "1.2.4" {
		t.Fatalf("default version = %q, want 1.2.4", version)
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
