package flow

import "testing"

func TestUnsupportedSourceIDsAreUnknown(t *testing.T) {
	for _, s := range []Source{2, 3, 4} {
		if got := s.String(); got != "unknown" {
			t.Errorf("unsupported source %d advertised as %q", s, got)
		}
	}
	if SrcSFlow != 1 || SrcCapture != 5 {
		t.Fatal("persisted source IDs changed")
	}
}
