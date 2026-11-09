//go:build linux

package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/auth"
	"github.com/githubflyideas/vyos-sflow/internal/bootstrap"
	"github.com/githubflyideas/vyos-sflow/internal/settings"
	"github.com/githubflyideas/vyos-sflow/internal/tui"
)

// This suite requires the real Linux DuckDB build. The portable session tests
// exercise orchestration but cannot replace these storage/socket assertions.
func TestTemporaryBackendAuthenticatedReadyAndCleanup(t *testing.T) {
	root := t.TempDir()
	var endpoint string
	err := runOwnedTUI(context.Background(), "127.0.0.1:0", "en", sessionHooks{
		tempRoot: root, backend: temporaryBackend,
		view: func(ctx context.Context, opt tui.Options) error {
			endpoint = opt.URL
			u, err := url.Parse(endpoint)
			if err != nil {
				t.Fatal(err)
			}
			if u.Hostname() != "127.0.0.1" || u.Port() == "0" {
				t.Errorf("unsafe bound URL %q", endpoint)
			}
			client := &http.Client{Timeout: 5 * time.Second}
			for _, test := range []struct {
				token  string
				status int
			}{{"", 401}, {opt.Token, 200}} {
				req, _ := http.NewRequestWithContext(ctx, "GET", endpoint+"/api/status", nil)
				if test.token != "" {
					req.Header.Set("Authorization", "Bearer "+test.token)
				}
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				resp.Body.Close()
				if resp.StatusCode != test.status {
					t.Errorf("status %d, want %d", resp.StatusCode, test.status)
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("owned data remains %v %v", entries, err)
	}
	u, _ := url.Parse(endpoint)
	c, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err == nil {
		c.Close()
		t.Fatal("HTTP listener survived session")
	}
}

func TestTemporaryBackendBusyUDPCleansOwnedData(t *testing.T) {
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	root := t.TempDir()
	err = runOwnedTUI(context.Background(), socket.LocalAddr().String(), "", sessionHooks{tempRoot: root, backend: temporaryBackend, view: func(context.Context, tui.Options) error { t.Error("view started despite UDP bind failure"); return nil }})
	if err == nil {
		t.Fatal("busy UDP unexpectedly started")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("failed startup left owned data")
	}
}

func TestPersistentServeKeepsFirstPasswordRestriction(t *testing.T) {
	dir := t.TempDir()
	cfg, err := settings.Parse([]string{"-data", dir, "-addr", "127.0.0.1:0", "-listen", "127.0.0.1:0"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan tui.Options, 1)
	done := make(chan error, 1)
	go func() { done <- serveConfigured(ctx, cfg, "", ready) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	var opt tui.Options
	select {
	case opt = <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("persistent service not ready")
	}
	req, _ := http.NewRequest("GET", opt.URL+"/api/overview?range=1h", nil)
	req.Header.Set("Authorization", "Bearer "+opt.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("factory password business status %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tui-token")); err != nil {
		t.Fatal(err)
	}
	checker, err := bootstrap.Ensure(dir, "admin", auth.Generate())
	if err != nil {
		t.Fatal(err)
	}
	if !checker.Check("admin", auth.DefaultPassword) {
		t.Fatal("existing account was overwritten")
	}
}
