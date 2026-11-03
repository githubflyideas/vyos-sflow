package tui

import (
	"strings"
	"testing"
)

func TestDeltaIsCapped(t *testing.T) {
	for _, c := range []struct {
		cur, base, tiny float64
		want            string
	}{
		{100, 50, 1, "+100%"}, {50, 100, 1, "-50%"}, {100, 100, 1, "±0%"},
		{6465390, 1000, 1, ">10×"}, // was shown as +646539%
		{100, 0.5, 1, "new"}, {0.5, 0, 1, ""}, {0, 0, 1, ""},
	} {
		if got := delta(c.cur, c.base, c.tiny); got != c.want {
			t.Errorf("delta(%v, %v) = %q, want %q", c.cur, c.base, got, c.want)
		}
	}
}

func TestNarrowDropsOptionalColumns(t *testing.T) {
	cols := []column{{head: "a"}, {head: "bar", width: 20, optional: true}, {head: "b", width: 5}}
	if c, keep := visibleCols(cols, 70); len(c) != 2 || keep[1] != 2 {
		t.Errorf("70 cols: %v %v", c, keep)
	}
	if c, _ := visibleCols(cols, 120); len(c) != 3 {
		t.Errorf("120 cols: %v", c)
	}
}

func TestFrameFitsAndHasNoWebHints(t *testing.T) {
	noColor = true
	defer func() { noColor = false }()
	a := newApp(Options{URL: "http://x"})
	a.data = page{sections: []section{{cols: []column{{head: "Host"}, {head: "bits/s", width: 11, right: true}, {head: "", width: 20, optional: true}},
		rows: []row{{key: "1", cells: []line{txt(strings.Repeat("very-long-host-name.", 6)), txt("1.00 Mbps"), styled(blue, "████")}}}}}}
	for _, w := range []int{70, 120} {
		a.w, a.h = w, 24
		lines := a.frame()
		if len(lines) != 24 {
			t.Fatalf("%d: %d lines", w, len(lines))
		}
		all := strings.Join(lines, "\n")
		if strings.Contains(all, "\x1b[3") {
			t.Errorf("%d: colour with NO_COLOR", w)
		}
		if strings.Contains(strings.ToLower(all), "click") {
			t.Errorf("%d: web-only hint in the TUI", w)
		}
		if !strings.Contains(all, "1.00 Mbps") {
			t.Errorf("%d: the rate was cut before the name", w)
		}
		if w == 70 && strings.Contains(all, "████") {
			t.Errorf("bar kept on a narrow terminal")
		}
	}
}

func TestWebURLMatchesView(t *testing.T) {
	a := newApp(Options{URL: "http://h:8066"})
	a.rng = 0
	a.pg = 1 // basic Top page
	u := a.webURL()
	for _, s := range []string{"v=topn", "dim=host", "r=15m"} {
		if !strings.Contains(u, s) {
			t.Errorf("%s lacks %s", u, s)
		}
	}
	a.pg = 3
	if u := a.webURL(); !strings.Contains(u, "v=sources") || strings.Contains(u, "dim=") {
		t.Errorf("sources: %s", u)
	}
}
