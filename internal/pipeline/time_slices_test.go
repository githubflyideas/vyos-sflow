package pipeline

import (
	"testing"
	"time"
)

func TestInstantSlicesKeepReceiveTime(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 12, 34, 30, 0, time.UTC)
	for _, delta := range []time.Duration{-time.Microsecond, 0, 15*time.Second + 123456*time.Microsecond} {
		at := cutoff.Add(delta)
		for _, start := range []time.Time{at, {}, at.Add(time.Second)} {
			got := minuteSlices(nil, start, at.In(time.FixedZone("offset", 9*3600)))
			if len(got) != 1 || !got[0].t.Equal(at) || got[0].frac != 1 {
				t.Fatalf("instant at %v: %+v; raw timestamp must not move across cutoff %v", at, got, cutoff)
			}
		}
	}
}

func TestSpanSlicesKeepAllocation(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 34, 30, 0, time.UTC)
	for _, span := range []time.Duration{90 * time.Second, 5 * time.Hour, 7 * time.Hour} {
		got := minuteSlices(nil, start, start.Add(span))
		var cum float64
		var done uint64
		for i, sl := range got {
			if !sl.t.Equal(sl.t.Truncate(time.Minute)) || sl.frac <= 0 {
				t.Fatalf("bad span slice: %+v", sl)
			}
			cum += sl.frac
			if i == len(got)-1 {
				cum = 1
			}
			done += share(17, cum, done)
		}
		if done != 17 {
			t.Fatalf("span %v allocated %d, want 17", span, done)
		}
	}
	got := minuteSlices(nil, start, start.Add(90*time.Second))
	if len(got) != 2 || got[0].frac != 1.0/3 || got[1].frac != 2.0/3 {
		t.Fatalf("overlap allocation: %+v", got)
	}
}
