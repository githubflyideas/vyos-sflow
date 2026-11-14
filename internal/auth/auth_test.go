package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPasswordContract(t *testing.T) {
	if DefaultPassword != "tryvyossflow66" {
		t.Fatalf("default password = %q, want tryvyossflow66", DefaultPassword)
	}
}

func TestSetLoadCheck(t *testing.T) {
	dir := t.TempDir()
	if err := Set(dir, "admin", "first"); err != nil {
		t.Fatal(err)
	}
	if err := Set(dir, "ops", "other"); err != nil {
		t.Fatal(err)
	}
	if err := Set(dir, "admin", "second"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(b), "second") || strings.Contains(string(b), "first") {
		t.Fatal("password stored in clear text")
	}
	if fi, _ := os.Stat(filepath.Join(dir, FileName)); fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("file mode %v, want owner only", fi.Mode().Perm())
	}
	es, err := Load(dir)
	if err != nil || len(es) != 2 {
		t.Fatalf("Load: %v %v", es, err)
	}
	c := NewChecker(nil, es)
	for _, tc := range []struct {
		u, p string
		ok   bool
	}{{"admin", "second", true}, {"admin", "second", true}, {"admin", "first", false}, {"ops", "other", true}, {"nobody", "x", false}, {"admin", "", false}} {
		if got := c.Check(tc.u, tc.p); got != tc.ok {
			t.Errorf("Check(%q, %q) = %v", tc.u, tc.p, got)
		}
	}
}

func TestPlainOverrides(t *testing.T) {
	c := NewChecker(map[string]string{"admin": "flag"}, []Entry{{"admin", Hash("file")}})
	if !c.Check("admin", "flag") || c.Check("admin", "file") {
		t.Fatal("a password given at start should replace the stored one")
	}
}

func TestLoadMissing(t *testing.T) {
	es, err := Load(t.TempDir())
	if es != nil || err != nil {
		t.Fatal(es, err)
	}
}

func TestFileCheckerSeesChanges(t *testing.T) {
	dir := t.TempDir()
	Set(dir, "admin", "one")
	fc := NewFileChecker(dir, nil)
	if !fc.Check("admin", "one") {
		t.Fatal("stored password rejected")
	}
	os.Remove(filepath.Join(dir, FileName))
	Set(dir, "admin", "two")
	if fc.Check("admin", "one") || !fc.Check("admin", "two") {
		t.Fatal("change of the password file not picked up")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	Set(dir, "admin", "a")
	Set(dir, "alice", "b")
	if err := Delete(dir, "nobody"); err == nil {
		t.Fatal("deleting an unknown user succeeded")
	}
	if err := Delete(dir, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := Delete(dir, "admin"); err != ErrLastUser {
		t.Fatalf("deleting the last user: %v", err)
	}
	es, _ := Load(dir)
	if len(es) != 1 || es[0].User != "admin" || !NewChecker(nil, es).Check("admin", "a") {
		t.Fatalf("%+v", es)
	}
}

// A name with no account takes as long to refuse as a wrong password, so
// the time a refusal takes does not tell which names exist.
func TestUnknownNameCostsAsMuchAsWrongPassword(t *testing.T) {
	c := NewChecker(nil, []Entry{{User: "admin", Hash: Hash("right-password-1")}})
	took := func(user string) time.Duration {
		best := time.Duration(1 << 62)
		for range 2 {
			start := time.Now()
			if c.Check(user, "wrong-password-1") {
				t.Fatal("accepted")
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	wrong, unknown := took("admin"), took("nobody")
	if unknown < wrong/2 {
		t.Fatalf("an unknown name is refused in %v, a wrong password in %v", unknown, wrong)
	}
}
