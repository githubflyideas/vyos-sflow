package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/githubflyideas/vyos-sflow/internal/tui"
)

func TestTUICommandAbsentServiceStartsOwnedSession(t *testing.T) {
	for _, args := range [][]string{nil, {"-listen", "127.0.0.1:0"}} {
		root := t.TempDir()
		started := false
		h := commandHooks{terminal: func() bool { return true }, getenv: func(string) string { return "" }, available: func(context.Context) (bool, error) { return false, nil },
			sessionHooks: sessionHooks{tempRoot: root,
				backend: func(ctx context.Context, dir, listen string, ready chan<- tui.Options) error {
					started = true
					want := "0.0.0.0:6343"
					if len(args) > 0 {
						want = "127.0.0.1:0"
					}
					if listen != want {
						t.Errorf("listen=%q, want %q", listen, want)
					}
					if filepath.Dir(dir) != root {
						t.Errorf("unowned directory %q", dir)
					}
					ready <- tui.Options{Token: "ephemeral"}
					<-ctx.Done()
					return nil
				}, view: func(context.Context, tui.Options) error { return nil },
			},
		}
		if err := runTUICommand(context.Background(), args, h); err != nil {
			t.Fatal(err)
		}
		if !started {
			t.Fatal("absent service did not start a session")
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Fatal("temporary data leaked")
		}
	}
}

func TestTUICommandRejectsNonTTYBeforeDiscovery(t *testing.T) {
	h := commandHooks{terminal: func() bool { return false }, getenv: func(string) string { return "" }, available: func(context.Context) (bool, error) { t.Error("discovery before terminal check"); return false, nil }}
	if err := runTUICommand(context.Background(), nil, h); err == nil {
		t.Fatal("non-TTY accepted")
	}
}

func TestTUICommandExplicitTargetNeverStartsBackend(t *testing.T) {
	for _, args := range [][]string{{"-url", "http://127.0.0.1:1"}, {"-data", t.TempDir()}, {"-user", "admin"}} {
		t.Run(args[0], func(t *testing.T) {
			failure := errors.New("unreachable")
			h := commandHooks{terminal: func() bool { return true }, getenv: func(string) string { return "password-for-test" },
				available: func(context.Context) (bool, error) { t.Error("explicit target discovery"); return false, nil },
				sessionHooks: sessionHooks{backend: func(context.Context, string, string, chan<- tui.Options) error {
					t.Error("explicit target started backend")
					return nil
				}, view: func(context.Context, tui.Options) error { return failure }},
			}
			if err := runTUICommand(context.Background(), args, h); err == nil {
				t.Fatal("attach failure hidden")
			}
		})
	}
}

func TestTUICommandExistingServiceRemainsUntouched(t *testing.T) {
	data := t.TempDir()
	tokenPath := filepath.Join(data, ".tui-token")
	os.WriteFile(tokenPath, []byte("existing-secret"), 0600)
	viewed := false
	h := commandHooks{terminal: func() bool { return true }, getenv: func(key string) string {
		if key == "VYOS_SFLOW_DATA" {
			return data
		}
		return ""
	},
		available: func(context.Context) (bool, error) { return true, nil },
		sessionHooks: sessionHooks{backend: func(context.Context, string, string, chan<- tui.Options) error {
			t.Error("started owned backend while attaching")
			return nil
		}, view: func(ctx context.Context, opt tui.Options) error {
			viewed = true
			if opt.Token != "existing-secret" {
				t.Errorf("token=%q", opt.Token)
			}
			return nil
		}},
	}
	if err := runTUICommand(context.Background(), nil, h); err != nil {
		t.Fatal(err)
	}
	if !viewed {
		t.Fatal("did not attach")
	}
	b, err := os.ReadFile(tokenPath)
	if err != nil || string(b) != "existing-secret" {
		t.Fatal("service token changed")
	}
}
