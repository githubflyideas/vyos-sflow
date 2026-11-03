package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/tui"
)

func TestOwnedSessionViewExitPathsReleaseSocketsAndData(t *testing.T) {
	for _, ending := range []string{"q", "EOF", "interrupt", "view-error"} {
		t.Run(ending, func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("initial API readiness failed")
			var address string
			err := runOwnedTUI(ctx, "127.0.0.1:0", "", sessionHooks{
				tempRoot: root,
				backend: func(ctx context.Context, dir, listen string, ready chan<- tui.Options) error {
					socket, err := net.ListenPacket("udp", listen)
					if err != nil {
						return err
					}
					defer socket.Close()
					address = socket.LocalAddr().String()
					ready <- tui.Options{}
					<-ctx.Done()
					return nil
				},
				view: func(ctx context.Context, _ tui.Options) error {
					switch ending {
					case "interrupt":
						cancel()
						<-ctx.Done()
					case "view-error":
						return failure
					}
					return nil // Both q and stdin EOF are clean view returns.
				},
			})
			if ending == "view-error" {
				if !errors.Is(err, failure) {
					t.Fatalf("error lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("owned data leaked")
			}
			socket, err := net.ListenPacket("udp", address)
			if err != nil {
				t.Fatalf("socket leaked: %v", err)
			}
			socket.Close()
		})
	}
}

func TestOwnedSessionPrematureBackendExitIsReadinessFailure(t *testing.T) {
	err := runOwnedTUI(context.Background(), "127.0.0.1:0", "", sessionHooks{
		tempRoot: t.TempDir(), backend: func(context.Context, string, string, chan<- tui.Options) error { return nil },
		view: func(context.Context, tui.Options) error { t.Error("view without readiness"); return nil },
	})
	if err == nil {
		t.Fatal("backend exited before readiness without an error")
	}
}

func TestOwnedSessionCancellationBeforeReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	root := t.TempDir()
	go func() {
		done <- runOwnedTUI(ctx, "127.0.0.1:0", "", sessionHooks{
			tempRoot: root, readyTimeout: time.Minute,
			backend: func(ctx context.Context, _ string, _ string, _ chan<- tui.Options) error {
				cancel()
				<-ctx.Done()
				return nil
			},
			view: func(context.Context, tui.Options) error { t.Error("view before ready"); return nil },
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("interrupt did not stop readiness wait")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("temporary data leaked")
	}
}

func TestOwnedSessionBackendFailureCancelsView(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("HTTP serve failed")
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runOwnedTUI(context.Background(), "127.0.0.1:0", "", sessionHooks{
			tempRoot: root,
			backend: func(ctx context.Context, _ string, _ string, ready chan<- tui.Options) error {
				ready <- tui.Options{}
				<-entered
				return failure
			},
			view: func(ctx context.Context, _ tui.Options) error { close(entered); <-ctx.Done(); return nil },
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("backend failure did not cancel view")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("temporary data leaked")
	}
}

func TestOwnedSessionReadinessTimeoutJoinsBackend(t *testing.T) {
	root := t.TempDir()
	stopped := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runOwnedTUI(context.Background(), "127.0.0.1:0", "", sessionHooks{
			tempRoot: root, readyTimeout: 10 * time.Millisecond,
			backend: func(ctx context.Context, _ string, _ string, _ chan<- tui.Options) error {
				<-ctx.Done()
				close(stopped)
				return nil
			},
			view: func(context.Context, tui.Options) error { t.Error("view before ready"); return nil },
		})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing readiness failure")
		}
	case <-time.After(time.Second):
		t.Fatal("readiness timeout not enforced")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("backend not joined")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("temporary data leaked")
	}
}

func TestOwnedSessionStartFailureCleansUp(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("UDP address already in use")
	done := make(chan error, 1)
	go func() {
		done <- runOwnedTUI(context.Background(), "127.0.0.1:0", "", sessionHooks{
			tempRoot: root, readyTimeout: time.Second,
			backend: func(context.Context, string, string, chan<- tui.Options) error { return failure },
			view:    func(context.Context, tui.Options) error { t.Error("view started on failed backend"); return nil },
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup failure hung waiting for readiness")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("temporary data leaked")
	}
}

func TestOwnedSessionRemovesOnlyItsDirectoryAfterBackendStops(t *testing.T) {
	parent := t.TempDir()
	sentinel := filepath.Join(parent, "existing")
	if err := os.WriteFile(sentinel, []byte("existing account/history"), 0600); err != nil {
		t.Fatal(err)
	}
	var owned string
	stopped := false
	err := runOwnedTUI(context.Background(), "127.0.0.1:0", "en", sessionHooks{
		tempRoot: parent, readyTimeout: time.Second,
		backend: func(ctx context.Context, dir, listen string, ready chan<- tui.Options) error {
			owned = dir
			if dir == parent || listen != "127.0.0.1:0" {
				t.Errorf("unsafe backend arguments %q %q", dir, listen)
			}
			ready <- tui.Options{URL: "http://127.0.0.1:1234", Token: "session"}
			<-ctx.Done()
			if _, err := os.Stat(dir); err != nil {
				t.Errorf("directory removed before backend stopped: %v", err)
			}
			stopped = true
			return nil
		},
		view: func(ctx context.Context, opt tui.Options) error {
			if opt.Token != "session" || opt.Lang != "en" {
				t.Errorf("options = %#v", opt)
			}
			return nil // q
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("backend not joined")
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned directory remains: %v", err)
	}
	b, err := os.ReadFile(sentinel)
	if err != nil || string(b) != "existing account/history" {
		t.Fatal("existing data changed")
	}
}
