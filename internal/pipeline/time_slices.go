package pipeline

import (
	"math"
	"time"
)

type slice struct {
	t    time.Time
	frac float64
}

// share is how much of total belongs to this minute: the rounded running
// total up to it, less what the minutes before it already got.
func share(total, cum float64, done uint64) uint64 {
	upTo := math.Round(total * min(cum, 1))
	if upTo <= float64(done) {
		return 0
	}
	return uint64(upTo) - done
}

// minuteSlices preserves the exact end time of instantaneous samples.
// Nonzero spans retain minute-aligned shares weighted by overlap.
func minuteSlices(out []slice, start, end time.Time) []slice {
	start, end = start.UTC(), end.UTC()
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if start.IsZero() || !start.Before(end) {
		return append(out, slice{end, 1})
	}
	if end.Sub(start) > 6*time.Hour { // implausible; keep the last hour
		start = end.Add(-time.Hour)
	}
	total := end.Sub(start).Seconds()
	for b := start.Truncate(time.Minute); b.Before(end); b = b.Add(time.Minute) {
		lo, hi := b, b.Add(time.Minute)
		if lo.Before(start) {
			lo = start
		}
		if hi.After(end) {
			hi = end
		}
		ov := hi.Sub(lo).Seconds()
		if ov <= 0 {
			continue
		}
		out = append(out, slice{b, ov / total})
	}
	if len(out) == 0 {
		return append(out, slice{end, 1})
	}
	return out
}
