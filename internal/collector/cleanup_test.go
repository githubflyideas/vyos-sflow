package collector

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestFailedClearPreservesReservoirAndDiagnostics(t *testing.T) {
	sink := &lifecycleSink{}
	c := New(sink)
	for i := uint32(1); i <= 11; i++ {
		sendLifecycle(t, c, 0, i, i*100, i*1000, "10.0.0.1", samplingEpoch.Add(time.Duration(i)*time.Second))
	}
	st := c.sf[sfKey{netip.MustParseAddr("192.0.2.1"), 0, 1}]
	before := *st
	denied := errors.New("clear failed")
	if err := c.ClearHistory(func() error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("error=%v", err)
	}
	if len(st.recent) != len(before.recent) || st.ref != before.ref || st.rate != before.rate || st.poolOK != before.poolOK {
		t.Fatal("failed clear changed reservoir/calibration")
	}
	sink.records = nil
	sendLifecycle(t, c, 0, 13, 1300, 13000, "10.0.0.2", samplingEpoch.Add(time.Minute))
	var oldWeight, total float64
	for _, r := range sink.records {
		total += r.Mult
		if r.Src.String() == "10.0.0.1" {
			oldWeight += r.Mult
		}
	}
	if oldWeight < 99.99 || total < 199.99 || total > 200.01 {
		t.Fatalf("failed clear lost compensation: old=%g total=%g", oldWeight, total)
	}
}
