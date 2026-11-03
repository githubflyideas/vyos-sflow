package settings

import (
	"path/filepath"
	"testing"
)

func TestDataDirectoryDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		env, want string
	}{
		{name: "default", want: "./vyso_sflow_data"},
		{name: "relative override", args: []string{"-data", "./other"}, want: "./other"},
		{name: "environment", env: "/srv/flow-history", want: "/srv/flow-history"},
		{name: "flag precedence", env: "/srv/flow-history", args: []string{"-data", "./chosen"}, want: "./chosen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Parse(tc.args, func(key string) string {
				if key == "VYOS_SFLOW_DATA" {
					return tc.env
				}
				return ""
			})
			if err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if want[0] != '/' {
				want, err = filepath.Abs(want)
				if err != nil {
					t.Fatal(err)
				}
			}
			if cfg.Data != want {
				t.Fatalf("data = %q, want %q", cfg.Data, want)
			}
		})
	}
	if _, err := Parse([]string{"-data", ""}, func(string) string { return "" }); err == nil {
		t.Fatal("empty data path accepted")
	}
}
