package settings

import "testing"

func TestAllInterfaceDefaults(t *testing.T) {
	c, err := Parse(nil, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "0.0.0.0:8066" || c.Listen != "0.0.0.0:6343" || c.DBThreads != 1 || c.RetentionDays != 2 || c.User != "admin" || c.TLSCert != "" || c.TLSKey != "" || c.AllowRemoteHTTP {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}
func TestEnvironmentAndFlagPrecedence(t *testing.T) {
	env := map[string]string{"VYOS_SFLOW_ADDR": "127.0.0.1:9999", "VYOS_SFLOW_LISTEN": "127.0.0.1:9998", "VYOS_SFLOW_RETENTION_DAYS": "2", "VYOS_SFLOW_DATA": "/srv/flows"}
	c, err := Parse([]string{"-addr", "127.0.0.1:8888", "-listen", "127.0.0.1:8887"}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:8888" || c.Listen != "127.0.0.1:8887" || c.RetentionDays != 2 || c.Data != "/srv/flows" {
		t.Fatalf("wrong precedence: %+v", c)
	}
}
func TestRejectInvalidSettings(t *testing.T) {
	for _, args := range [][]string{{"-memory", "NaN"}, {"-memory", "-1"}, {"-retention-days", "0"}, {"-db-threads", "0"}, {"-listen", "sflow=:6343,netflow=:2055"}, {"-listen", "127.0.0.1:99999"}, {"-tls-cert", "only-cert"}, {"-tls-key", "only-key"}, {"stray"}} {
		if _, err := Parse(args, func(string) string { return "" }); err == nil {
			t.Errorf("accepted invalid options %v", args)
		}
	}
}
func TestRemoteHTTPNeedsNoOptIn(t *testing.T) {
	for _, addr := range []string{":8066", "0.0.0.0:8066", "192.0.2.1:8066", "[::]:8066", "127.0.0.1:8066"} {
		for _, legacy := range []string{"", "true", "false"} {
			env := map[string]string{"VYOS_SFLOW_ADDR": addr, "VYOS_SFLOW_ALLOW_REMOTE_HTTP": legacy}
			if _, err := Parse(nil, func(k string) string { return env[k] }); err != nil {
				t.Errorf("address %s, legacy %q: %v", addr, legacy, err)
			}
			args := []string{"-addr", addr}
			if legacy != "" {
				args = append(args, "-allow-remote-http="+legacy)
			}
			if _, err := Parse(args, func(string) string { return "" }); err != nil {
				t.Errorf("flags %v: %v", args, err)
			}
		}
	}
}
func TestBadEnvironmentDoesNotSilentlyDefault(t *testing.T) {
	if _, err := Parse(nil, func(k string) string {
		if k == "VYOS_SFLOW_DB_THREADS" {
			return "not-an-integer"
		}
		return ""
	}); err == nil {
		t.Fatal("malformed environment accepted")
	}
}
