package bootstrap

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/auth"
	"os"
	"path/filepath"
	"testing"
)

func TestInitialCredentialIsDefaultWithoutPlaintextFile(t *testing.T) {
	d := t.TempDir()
	c, err := Ensure(d, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || !c.Check("admin", "tryvyossflow66") || !c.HasDefault() {
		t.Fatal("fresh installation must accept admin/tryvyossflow66 and require change")
	}
	if _, err := os.Stat(filepath.Join(d, "initial-password")); !os.IsNotExist(err) {
		t.Fatalf("plaintext initial-password file must not exist: %v", err)
	}
}
func TestRestartDoesNotReplaceExistingPassword(t *testing.T) {
	d := t.TempDir()
	if err := auth.Set(d, "admin", "original-secure-password"); err != nil {
		t.Fatal(err)
	}
	c, err := Ensure(d, "admin", "replacement-password")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || !c.Check("admin", "original-secure-password") || c.Check("admin", "replacement-password") {
		t.Fatal("restart changed persisted credential")
	}
}
func TestRejectWeakInitialPassword(t *testing.T) {
	if _, err := Ensure(t.TempDir(), "admin", "short"); err == nil {
		t.Fatal("weak initial password accepted")
	}
}
