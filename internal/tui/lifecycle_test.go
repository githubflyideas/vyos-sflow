package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInputEOFAndQuitKeys(t *testing.T) {
	for _, input := range []string{"q", "\x03", "\x04", ""} {
		t.Run(input, func(t *testing.T) {
			keys := make(chan string, 16)
			readKeysFrom(context.Background(), strings.NewReader(input), keys)
			a := newApp(Options{})
			for k := range keys {
				a.key(k)
			}
			if input != "" && !a.quit {
				t.Fatalf("input %q did not quit", input)
			}
		})
	}
}

func TestAPIRequestUsesSessionCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := newApp(Options{URL: server.URL})
	a.ctx = ctx
	done := make(chan error, 1)
	go func() { var value any; done <- a.get("status", nil, &value) }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("request succeeded after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("session cancellation left API request running")
	}
}

func TestQuitKeyExitsEvenWithHelpOpen(t *testing.T) {
	a := newApp(Options{})
	a.help = true
	a.key("q")
	if !a.quit {
		t.Fatal("q only dismissed help instead of ending session")
	}
}

func TestTerminalPreflightExported(t *testing.T) { _ = IsTerminal() }
