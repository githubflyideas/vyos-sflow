package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/githubflyideas/vyos-sflow/internal/bootstrap"
	"github.com/githubflyideas/vyos-sflow/internal/settings"
)

func TestDefaultDirectoryRestartPreservesUserData(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	parse := func() settings.Config {
		t.Helper()
		c, err := settings.Parse(nil, func(string) string { return "" })
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := parse()
	if c.Data != filepath.Join(cwd, "vyso_sflow_data") {
		t.Fatalf("wrong default path %s", c.Data)
	}
	if _, err := bootstrap.Ensure(c.Data, "admin", "original-secure-password"); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(c.Data, "history-sentinel")
	if err := os.WriteFile(sentinel, []byte("existing history"), 0600); err != nil {
		t.Fatal(err)
	}
	c = parse()
	accounts, err := bootstrap.Ensure(c.Data, "admin", "replacement-password")
	if err != nil {
		t.Fatal(err)
	}
	if !accounts.Check("admin", "original-secure-password") || accounts.Check("admin", "replacement-password") {
		t.Fatal("restart replaced account")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "existing history" {
		t.Fatalf("restart changed existing file: %q %v", b, err)
	}
}
