package settings

import "testing"

func TestNoIdentityParameters(t *testing.T) {
	empty := func(string) string { return "" }
	for _, args := range [][]string{{"-agent", "192.0.2.1"}, {"-sub-agent", "100000"}} {
		if _, err := Parse(args, empty); err == nil {
			t.Errorf("removed identity parameter accepted: %v", args)
		}
	}
	env := map[string]string{"VYOS_SFLOW_AGENT": "invalid-old-value", "VYOS_SFLOW_SUB_AGENT": "invalid-old-value", "VYOS_SFLOW_DATA": "/config/existing", "VYOS_SFLOW_ACCEPT_48H_RETENTION": "true"}
	c, err := Parse(nil, func(k string) string { return env[k] })
	if err != nil || c.Data != "/config/existing" || !c.Accept48hRetention {
		t.Fatalf("legacy environment: %+v %v", c, err)
	}
	for _, days := range []string{"1", "3", "3650"} {
		if _, err := Parse([]string{"-retention-days", days}, empty); err == nil {
			t.Errorf("accepted retention %s", days)
		}
	}
}
