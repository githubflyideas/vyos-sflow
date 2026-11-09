package bootstrap

import (
	"fmt"
	"github.com/githubflyideas/vyos-sflow/internal/auth"
	"os"
)

// Ensure seeds an empty installation only. Existing accounts always win over
// environment variables, so an unattended restart cannot reset a password.
func Ensure(dir, user, initial string) (*auth.FileChecker, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	entries, err := auth.Load(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return auth.NewFileChecker(dir, nil), nil
	}
	if initial == "" {
		initial = auth.DefaultPassword
	}
	if len(initial) < 12 {
		return nil, fmt.Errorf("initial password must contain at least 12 characters")
	}
	if err := auth.Set(dir, user, initial); err != nil {
		return nil, err
	}
	return auth.NewFileChecker(dir, nil), nil
}
