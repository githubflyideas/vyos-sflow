package sflow

import (
	"testing"
	"time"
)

func TestCounterMandatoryFields(t *testing.T) {
	// Generic interface counters have 88 mandatory octets, including
	// outDiscards, outErrors and promiscuousMode after outBroadcastPkts.
	for n := 0; n < 88; n++ {
		body := append(words(1, 1, 1, 1, uint32(n)), make([]byte, n)...)
		d, err := Decode(samplePacket(2, body), time.Time{})
		if err == nil {
			t.Errorf("accepted %d of 88 mandatory counter bytes", n)
		}
		if len(d.Counters) != 0 {
			t.Errorf("published partial counter at length %d", n)
		}
	}
	d, err := Decode(samplePacket(2, append(words(1, 1, 1, 1, 88), make([]byte, 88)...)), time.Time{})
	if err != nil || len(d.Counters) != 1 {
		t.Fatalf("complete generic counter rejected: %v", err)
	}
}
func TestCounterSampleHeaderTruncation(t *testing.T) {
	for _, format := range []uint32{2, 4} {
		size := 12
		if format == 4 {
			size = 16
		}
		for n := 0; n < size; n++ {
			if _, err := Decode(samplePacket(format, make([]byte, n)), time.Time{}); err == nil {
				t.Errorf("accepted format %d sample header of %d bytes", format, n)
			}
		}
	}
}
