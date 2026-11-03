package api

import (
	"errors"
	"net/url"
	"strconv"
	"time"
)

const maxHistory = 48 * time.Hour

var ranges = map[string]time.Duration{"5m": 5 * time.Minute, "15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "48h": maxHistory}

// historyWindow bounds queries, not retention; no stored data is deleted.
func historyWindow(v url.Values, now time.Time) (from, to time.Time, err error) {
	now = now.UTC().Truncate(time.Millisecond)
	to = now
	if key := v.Get("range"); v.Has("range") {
		if _, ok := ranges[key]; !ok {
			return from, to, errors.New("unsupported range; maximum is 48h")
		}
	}
	if f, t := v.Get("from"), v.Get("to"); v.Has("from") || v.Has("to") {
		fm, e1 := strconv.ParseInt(f, 10, 64)
		tm, e2 := strconv.ParseInt(t, 10, 64)
		if e1 != nil || e2 != nil || tm <= fm {
			return from, to, errors.New("bad from/to")
		}
		from, to = time.UnixMilli(fm).UTC(), time.UnixMilli(tm).UTC()
	} else {
		key := v.Get("range")
		if key == "" {
			key = "1h"
		}
		d, ok := ranges[key]
		if !ok {
			return from, to, errors.New("unsupported range; maximum is 48h")
		}
		from = now.Add(-d)
	}
	if from.Before(now.Add(-maxHistory)) || to.After(now) || to.Sub(from) > maxHistory {
		return from, to, errors.New("history must be within the last 48 hours")
	}
	return
}
