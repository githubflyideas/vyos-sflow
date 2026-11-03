// Package tui provides five basic pages over the local authenticated API.
package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/githubflyideas/vyos-sflow-new/internal/web"
)

// Options configure the TUI.
type Options struct {
	URL, Token, User, Password, Lang string
}

// ------------------------------------------------------------ i18n

type dict map[string]string

func loadDict(lang string) (dict, dict) {
	read := func(code string) dict {
		b, err := io.ReadAll(must(web.FS().Open("i18n/" + code + ".json")))
		if err != nil {
			return dict{}
		}
		d := dict{}
		json.Unmarshal(b, &d)
		return d
	}
	return read(lang), read("en")
}

func must(f interface{ Read([]byte) (int, error) }, err error) io.Reader {
	if err != nil {
		return strings.NewReader("{}")
	}
	return f
}

func detectLang(explicit string) string {
	cands := []string{explicit, os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"), os.Getenv("LANGUAGE")}
	for _, c := range cands {
		c = strings.ToLower(c)
		if len(c) < 2 {
			continue
		}
		code := c[:2]
		for _, l := range []string{"en", "zh", "hi", "es", "ar", "fr", "bn", "pt", "ru", "id", "ur", "de", "ja", "vi", "ko"} {
			if code == l {
				return l
			}
		}
	}
	return "en"
}

// ------------------------------------------------------------ styles

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	rev    = "\x1b[7m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	cyan   = "\x1b[36m"
)

// noColor follows https://no-color.org: colours are dropped, while bold,
// dim and reverse video (which carry no colour) stay.
var noColor = os.Getenv("NO_COLOR") != ""

func stripColor(st string) string {
	if !noColor {
		return st
	}
	for _, c := range []string{red, green, yellow, blue, cyan} {
		st = strings.ReplaceAll(st, c, "")
	}
	return st
}

type seg struct{ style, text string }

type line []seg

func (l line) width() int {
	w := 0
	for _, s := range l {
		w += strWidth(s.text)
	}
	return w
}

// render pads or cuts the line to w cells.
func (l line) render(w int, base string) string {
	var b strings.Builder
	used := 0
	for _, s := range l {
		if used >= w {
			break
		}
		t := s.text
		if sw := strWidth(t); used+sw > w {
			t = fit(t, w-used)
		}
		b.WriteString(base + stripColor(s.style) + t + reset)
		used += strWidth(t)
	}
	b.WriteString(base + spaces(w-used) + reset)
	return b.String()
}

// plain is the line's text without styles.
func (l line) plain() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.text)
	}
	return b.String()
}

// ------------------------------------------------------------ model

type row struct {
	cells  []line
	key    string // stable identity (keeps the selection across refreshes)
	detail []line // shown under the row when it is expanded
}

type column struct {
	head     string
	width    int  // fixed width; 0 = flexible
	right    bool // right-aligned
	optional bool // dropped first on narrow terminals
}

type section struct {
	title line
	cols  []column
	rows  []row
}

type page struct {
	top      []line // fixed lines above the lists
	sections []section
	foot     []line // fixed lines below the lists
	err      string
}

var pageKeys = []string{"overview", "top", "traffic", "sources", "help"}
var ranges = []string{"15m", "1h", "6h", "24h", "48h"}
var rangeDur = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour, 48 * time.Hour}
var dims = []string{"host", "app", "proto"}
var units = []string{"bps", "bytes", "pps"}
var intervals = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}

// narrow is the width below which optional columns are dropped.
const narrow = 80

type app struct {
	ctx    context.Context
	opt    Options
	tr, en dict
	cl     *http.Client

	mu       sync.Mutex
	pg       int
	rng      int
	dim      int
	unit     int
	ival     int
	paused   bool
	sel      int
	scroll   int
	data     page
	loading  bool
	updated  time.Time
	help     bool
	msg      string
	msgUntil time.Time
	selKey   string
	w, h     int
	quit     bool
	refresh  chan struct{}
}

func (a *app) t(k string, kv ...string) string {
	s, ok := a.tr[k]
	if !ok {
		s, ok = a.en[k]
		if !ok {
			s = k
		}
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

func newApp(opt Options) *app {
	tr, en := loadDict(detectLang(opt.Lang))
	return &app{opt: opt, tr: tr, en: en, cl: &http.Client{Timeout: 20 * time.Second}, rng: 1, ival: 1,
		refresh: make(chan struct{}, 1), w: 120, h: 36}
}

// ------------------------------------------------------------ run

// IsTerminal permits callers to reject redirected input before starting resources.
func IsTerminal() bool { return isTerminal() }

// Run starts the terminal UI and blocks until the user quits or ctx ends.
func Run(ctx context.Context, opt Options) error {
	if !isTerminal() {
		return errors.New("vyos-sflow tui needs an interactive terminal")
	}
	if opt.URL == "" {
		opt.URL = "http://127.0.0.1:8066"
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a := newApp(opt)
	a.ctx = ctx
	// first contact, before switching the terminal into raw mode
	var st map[string]any
	if err := a.get("status", nil, &st); err != nil {
		if strings.Contains(err.Error(), "401") {
			return errors.New(a.t("tui.login"))
		}
		return errors.New(a.t("tui.error", "url", opt.URL, "e", err.Error()))
	}
	raw, err := makeRaw()
	if err != nil {
		return err
	}
	defer raw.restore()
	out := bufio.NewWriterSize(os.Stdout, 1<<16)
	out.WriteString("\x1b[?1049h\x1b[?25l")
	out.Flush()
	defer func() {
		os.Stdout.WriteString("\x1b[?25h\x1b[?1049l")
	}()

	keys := make(chan string, 16)
	waitKeys := startKeys(ctx, keys)
	a.w, a.h = size()
	fetchDone := make(chan struct{})
	go func() { defer close(fetchDone); a.fetchLoop(ctx) }()
	defer func() { cancel(); waitKeys(); <-fetchDone }()
	a.kick()

	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for !a.quit {
		a.draw(out)
		select {
		case <-ctx.Done():
			return nil
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			a.key(k)
		case <-tick.C:
			a.w, a.h = size()
		}
	}
	return nil
}

func (a *app) kick() {
	select {
	case a.refresh <- struct{}{}:
	default:
	}
}

func (a *app) fetchLoop(ctx context.Context) {
	for {
		a.mu.Lock()
		iv := intervals[a.ival]
		a.mu.Unlock()
		t := time.NewTimer(iv)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-a.refresh:
			t.Stop()
		case <-t.C:
			a.mu.Lock()
			busy := a.paused || a.help
			a.mu.Unlock()
			if busy {
				continue
			}
		}
		a.reload()
	}
}

// reload fetches the current page once.
func (a *app) reload() {
	a.mu.Lock()
	a.loading = true
	st := a.state()
	a.mu.Unlock()
	p := a.load(st)
	a.mu.Lock()
	if st == a.state() {
		a.data = p
		a.updated = time.Now()
		a.restoreSel()
	}
	a.loading = false
	a.mu.Unlock()
}

// view is everything a page's content depends on.
type view struct{ pg, rng, dim, unit, w int }

func (a *app) state() view { return view{a.pg, a.rng, a.dim, a.unit, a.w} }

func (a *app) restoreSel() {
	n := a.rowCount()
	if a.selKey != "" {
		i := 0
		for _, s := range a.data.sections {
			for _, r := range s.rows {
				if r.key == a.selKey {
					a.sel = i
					return
				}
				i++
			}
		}
	}
	if a.sel >= n {
		a.sel = max(0, n-1)
	}
	a.rememberSel()
}

func (a *app) rememberSel() {
	if r := a.selRow(); r != nil {
		a.selKey = r.key
	} else {
		a.selKey = ""
	}
}

// ------------------------------------------------------------ http

func (a *app) get(path string, q url.Values, v any) error {
	return a.do("GET", path, q, nil, v)
}

func (a *app) do(method, path string, q url.Values, body io.Reader, v any) error {
	u := a.opt.URL + "/api/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if a.opt.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.opt.Token)
	} else if a.opt.User != "" {
		req.SetBasicAuth(a.opt.User, a.opt.Password)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		var e struct{ Error string }
		json.NewDecoder(res.Body).Decode(&e)
		return fmt.Errorf("%d %s", res.StatusCode, e.Error)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

func query(rng int, extra map[string]string) url.Values {
	q := url.Values{"range": {ranges[rng]}}
	for k, v := range extra {
		q.Set(k, v)
	}
	return q
}

// ------------------------------------------------------------ keys

func readKeys(out chan<- string) {
	readKeysFrom(context.Background(), os.Stdin, out)
}

func readKeysFrom(ctx context.Context, input io.Reader, out chan<- string) {
	defer close(out)
	r := bufio.NewReader(input)
	send := func(key string) bool {
		select {
		case out <- key:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		if ctx.Err() != nil {
			return
		}
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		key := ""
		switch {
		case b == 0x1b:
			time.Sleep(15 * time.Millisecond)
			if r.Buffered() == 0 {
				key = "esc"
				break
			}
			b2, _ := r.ReadByte()
			if b2 != '[' && b2 != 'O' {
				key = "esc"
				break
			}
			seq := ""
			for len(seq) < 32 {
				c, err := r.ReadByte()
				if err != nil {
					return
				}
				seq += string(c)
				if (c >= 'A' && c <= 'Z') || c == '~' || (c >= 'a' && c <= 'z') {
					break
				}
			}
			key = map[string]string{"A": "up", "B": "down", "C": "right", "D": "left", "H": "home", "1~": "home", "F": "end", "4~": "end", "5~": "pgup", "6~": "pgdn", "3~": "delete", "Z": "backtab"}[seq]
		case b == '\r' || b == '\n':
			key = "enter"
		case b == 0x7f || b == 0x08:
			key = "backspace"
		case b == '	':
			key = "tab"
		case b == 3 || b == 4:
			key = "ctrl-c"
		case b < 0x20:
		default:
			buf := []byte{b}
			for !utf8.FullRune(buf) {
				c, err := r.ReadByte()
				if err != nil {
					return
				}
				buf = append(buf, c)
			}
			key = string(buf)
		}
		if key != "" && !send(key) {
			return
		}
	}
}

func (a *app) changed() {
	a.resetSel()
	a.kick()
}

func (a *app) key(k string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if k == "ctrl-c" || k == "q" {
		a.quit = true
		return
	}
	if a.help {
		a.help = false
		return
	}
	n := a.rowCount()
	page := pageKeys[a.pg]
	switch k {
	case "q":
		a.quit = true
	case "up", "k":
		if a.sel > 0 {
			a.sel--
		}
	case "down", "j":
		if a.sel < n-1 {
			a.sel++
		}
	case "pgup":
		a.sel = max(0, a.sel-max(1, a.h-8))
	case "pgdn":
		a.sel = min(max(0, n-1), a.sel+max(1, a.h-8))
	case "home", "g":
		a.sel = 0
	case "end", "G":
		a.sel = max(0, n-1)
	case "left", "right", "h", "l":
		back := k == "left" || k == "h"
		switch page {
		case "top":
			d := 1
			if back {
				d = len(dims) - 1
			}
			a.dim = (a.dim + d) % len(dims)
			a.changed()
		}
	case "tab", "backtab":
		d := 1
		if k == "backtab" {
			d = len(pageKeys) - 1
		}
		a.setPage((a.pg + d) % len(pageKeys))
	case "1", "2", "3", "4", "5":
		a.setPage(int(k[0] - '1'))
	case "t":
		a.rng = (a.rng + 1) % len(ranges)
		a.changed()
	case "T":
		a.rng = (a.rng + len(ranges) - 1) % len(ranges)
		a.changed()
	case "u":
		a.unit = (a.unit + 1) % len(units)
		a.kick()
	case "p", " ":
		a.paused = !a.paused
		if !a.paused {
			a.kick()
		}
	case "+", "=":
		a.ival = min(len(intervals)-1, a.ival+1)
		a.kick()
	case "-", "_":
		a.ival = max(0, a.ival-1)
		a.kick()
	case "r":
		a.kick()
	case "?":
		a.help = true
	case "w":
		a.openBrowser()
	}

	a.rememberSel()
}

func (a *app) setPage(p int) {
	if p == a.pg {
		return
	}
	a.pg = p
	a.data = page{}
	a.selKey = ""
	a.changed()
}

func (a *app) resetSel() { a.sel, a.scroll, a.selKey = 0, 0, "" }

func (a *app) flash(s string) {
	a.msg, a.msgUntil = s, time.Now().Add(3*time.Second)
}

// webURL is the web UI address of the view the TUI shows.
func (a *app) webURL() string {
	v := map[string]string{"overview": "overview", "top": "topn", "traffic": "traffic", "sources": "sources", "help": "help"}[pageKeys[a.pg]]
	r := ranges[a.rng]
	h := url.Values{"v": {v}, "r": {r}}
	if pageKeys[a.pg] == "top" {
		h.Set("dim", dims[a.dim])
	}
	return a.opt.URL + "/#" + h.Encode()
}

func (a *app) openBrowser() {
	u := a.webURL()
	if err := OpenBrowser(u); err != nil {
		a.flash(a.t("tui.open_failed", "url", u))
		return
	}
	a.flash(a.t("tui.opened"))
}

// OpenBrowser opens u in the default web browser.
func OpenBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

// ------------------------------------------------------------ drawing

func (a *app) rowCount() int {
	n := 0
	for _, s := range a.data.sections {
		n += len(s.rows)
	}
	return n
}

func (a *app) selRow() *row {
	i := a.sel
	for si := range a.data.sections {
		s := &a.data.sections[si]
		if i < len(s.rows) {
			return &s.rows[i]
		}
		i -= len(s.rows)
	}
	return nil
}

func (a *app) draw(out *bufio.Writer) {
	a.mu.Lock()
	lines := a.frame()
	a.mu.Unlock()
	out.WriteString("\x1b[H")
	for i, l := range lines {
		out.WriteString(l)
		if i < len(lines)-1 {
			out.WriteString("\r\n")
		}
	}
	out.Flush()
}

// frame lays out the whole screen, one string per terminal row.
func (a *app) frame() []string {
	w, h := a.w, a.h
	if w < 40 || h < 12 {
		return []string{"vyos-sflow: window too small"}
	}
	var lines []string
	emit := func(l line) { lines = append(lines, l.render(w, "")) }

	// status line: pages, range, units, refresh
	hdr := line{{bold + blue, "vyos-sflow"}, {"", " "}}
	for i, k := range pageKeys {
		lab := fmt.Sprintf(" %d %s ", i+1, a.t("nav."+map[string]string{"top": "topn", "overview": "overview", "traffic": "traffic", "sources": "sources", "help": "help"}[k]))
		if i == a.pg {
			hdr = append(hdr, seg{rev + bold, lab})
		} else {
			hdr = append(hdr, seg{dim, lab})
		}
	}
	hdr = append(hdr, seg{"", "  " + a.t("range."+ranges[a.rng]) + " · " + a.t("tui.unit."+units[a.unit]) + " · " + a.t("tui.every", "s", fmt.Sprint(intervals[a.ival].Seconds()))})
	if a.paused {
		hdr = append(hdr, seg{bold + yellow, "  ‖ " + a.t("tui.paused")})
	} else if a.loading {
		hdr = append(hdr, seg{dim, "  …"})
	}
	if !a.updated.IsZero() && w >= narrow {
		hdr = append(hdr, seg{dim, "  " + a.t("tui.updated", "t", a.updated.Format("15:04:05"))})
	}
	emit(hdr)
	// body
	body := h - len(lines) - 1
	var bl []string
	if a.data.err != "" {
		bl = append(bl, line{{red, a.data.err}}.render(w, ""))
	}
	for _, l := range a.data.top {
		bl = append(bl, l.render(w, ""))
	}
	type rl struct {
		text string
		idx  int // row index, -1 for headers and details
	}
	var list []rl
	ri := 0
	for _, s := range a.data.sections {
		cols, keep := visibleCols(s.cols, w)
		widths := layout(cols, w-1)
		if len(s.title) > 0 {
			list = append(list, rl{s.title.render(w, ""), -1})
		}
		hd := line{{"", " "}}
		for i, c := range cols {
			txt := c.head
			if c.right {
				txt = fitRight(txt, widths[i])
			} else {
				txt = fit(txt, widths[i])
			}
			hd = append(hd, seg{dim, txt + " "})
		}
		list = append(list, rl{hd.render(w, ""), -1})
		if len(s.rows) == 0 {
			list = append(list, rl{line{{dim, " " + a.t("tui.nothing")}}.render(w, ""), -1})
		}
		for _, r := range s.rows {
			l := line{{"", " "}}
			if ri == a.sel {
				l = line{{bold, "›"}}
			}
			for i, ci := range keep {
				var c line
				if ci < len(r.cells) {
					c = r.cells[ci]
				}
				cw := c.width()
				if cw > widths[i] {
					st := ""
					if len(c) > 0 {
						st = c[0].style
					}
					c = line{{st, fit(c.plain(), widths[i])}}
				} else if cols[i].right {
					c = append(line{{"", spaces(widths[i] - cw)}}, c...)
				} else {
					c = append(c, seg{"", spaces(widths[i] - cw)})
				}
				l = append(l, c...)
				l = append(l, seg{"", " "})
			}
			base := ""
			if ri == a.sel {
				base = rev
			}
			list = append(list, rl{l.render(w, base), ri})
			for _, d := range r.detail {
				list = append(list, rl{d.render(w, ""), -1})
			}
			ri++
		}
		list = append(list, rl{spaces(w), -1})
	}
	var foot []string
	for _, l := range a.data.foot {
		foot = append(foot, l.render(w, ""))
	}
	avail := max(3, body-len(bl)-len(foot))
	// keep the selected row (and what is expanded under it) visible
	selPos, selEnd := 0, 0
	for i, x := range list {
		if x.idx == a.sel {
			selPos, selEnd = i, i
			for j := i + 1; j < len(list) && list[j].idx == -1 && j-i <= avail-2; j++ {
				selEnd = j
			}
			break
		}
	}
	if selPos < a.scroll+1 {
		a.scroll = max(0, selPos-1)
	}
	if selEnd >= a.scroll+avail {
		a.scroll = selEnd - avail + 1
	}
	if a.scroll > len(list)-avail {
		a.scroll = max(0, len(list)-avail)
	}

	for i := a.scroll; i < len(list) && i < a.scroll+avail; i++ {
		bl = append(bl, list[i].text)
	}
	for len(bl) < body-len(foot) {
		bl = append(bl, spaces(w))
	}
	bl = append(bl, foot...)
	lines = append(lines, bl[:min(len(bl), body)]...)

	// footer
	switch {
	case a.msg != "" && time.Now().Before(a.msgUntil):
		lines = append(lines, line{{green, a.msg}}.render(w, ""))
	default:
		keys := "1–5 Tab   t T   u   r   p   w   ?   q"
		if strWidth(keys) > w {
			keys = "? " + a.t("tui.help_title") + "   q " + a.t("tui.h.quit")
		}
		lines = append(lines, line{{dim, keys}}.render(w, ""))
	}

	if a.help {
		lines = overlay(lines, a.helpBox(), 3, 2, min(w-4, 80))
	}
	return lines
}

// overlay draws a framed box over the lines at row top, column left.
func overlay(lines []string, box []line, top, left, mw int) []string {
	put := func(y int, s string) {
		if y >= 0 && y < len(lines) {
			lines[y] = s
		}
	}
	edge := strings.Repeat("─", mw)
	put(top, spaces(left)+"┌"+edge+"┐")
	for i, b := range box {
		put(top+1+i, spaces(left)+"│"+b.render(mw, "")+"│")
	}
	put(top+1+len(box), spaces(left)+"└"+edge+"┘")
	return lines
}

func (a *app) helpBox() []line {
	box := []line{{{bold, a.t("tui.help_title")}}}
	for _, k := range []struct{ keys, text string }{
		{"1–5  Tab", "tui.h.pages"}, {"↑↓ PgUp PgDn", "tui.h.move"}, {"← →", "tui.h.dim"},
		{"t  T", "tui.h.range"}, {"u", "tui.h.units"},
		{"+  -", "tui.h.refresh"}, {"p", "tui.h.pause"}, {"w", "tui.h.web"}, {"q", "tui.h.quit"},
	} {
		box = append(box, line{{bold, fit(k.keys, 14)}, {"", " " + a.t(k.text)}})
	}
	box = append(box, line{}, line{{dim, a.t("tui.close")}})
	return box
}

// visibleCols drops the optional columns on narrow terminals, so that text
// is cut only when nothing else can go. keep maps back to the cell index.
func visibleCols(cols []column, w int) ([]column, []int) {
	var out []column
	var keep []int
	for i, c := range cols {
		if c.optional && w < narrow {
			continue
		}
		out = append(out, c)
		keep = append(keep, i)
	}
	return out, keep
}

// layout gives each column a width; flexible columns share what is left.
func layout(cols []column, total int) []int {
	ws := make([]int, len(cols))
	fixed, flex := 0, 0
	for i, c := range cols {
		ws[i] = c.width
		if c.width == 0 {
			flex++
		}
		fixed += c.width + 1
	}
	rest := total - fixed
	for i, c := range cols {
		if c.width == 0 {
			ws[i] = max(6, rest/max(1, flex)-1)
		}
	}
	return ws
}

// ------------------------------------------------------------ formatting

func fmtBps(v float64) string {
	for _, u := range []struct {
		n string
		m float64
	}{{"Gbps", 1e9}, {"Mbps", 1e6}, {"Kbps", 1e3}} {
		if math.Abs(v) >= u.m {
			x := v / u.m
			if math.Abs(x) < 10 {
				return fmt.Sprintf("%.2f %s", x, u.n)
			}
			return fmt.Sprintf("%.1f %s", x, u.n)
		}
	}
	return fmt.Sprintf("%.0f bps", v)
}

func fmtBytes(v float64) string {
	for _, u := range []struct {
		n string
		m float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"KB", 1e3}} {
		if v >= u.m {
			x := v / u.m
			if x < 10 {
				return fmt.Sprintf("%.2f %s", x, u.n)
			}
			return fmt.Sprintf("%.1f %s", x, u.n)
		}
	}
	return fmt.Sprintf("%.0f B", v)
}

func fmtPps(v float64) string {
	for _, u := range []struct {
		n string
		m float64
	}{{"Gpps", 1e9}, {"Mpps", 1e6}, {"Kpps", 1e3}} {
		if v >= u.m {
			return fmt.Sprintf("%.1f %s", v/u.m, u.n)
		}
	}
	if v < 10 {
		return fmt.Sprintf("%.1f pps", v)
	}
	return fmt.Sprintf("%.0f pps", v)
}

func pct(v float64) string {
	s := fmt.Sprintf("%.1f%%", v*100)
	if v > 0 {
		s = "+" + s
	}
	return s
}

// delta formats the change from base to cur. It is empty when the base is
// too small to compare against (tiny is the floor), and capped, so that a
// host that was nearly idle reads "new" or ">10×" rather than "+646539%".
func delta(cur, base, tiny float64) string {
	switch {
	case base <= 0 && cur <= tiny:
		return ""
	case base <= tiny:
		if cur > tiny {
			return "new"
		}
		return ""
	case cur/base > 10:
		return ">10×"
	}
	d := (cur - base) / base
	if math.Abs(d) < 0.005 {
		return "±0%"
	}
	if d > 0 {
		return fmt.Sprintf("+%.0f%%", d*100)
	}
	return fmt.Sprintf("%.0f%%", d*100)
}

func barCells(v, maxv float64, width int) string {
	if maxv <= 0 || width <= 0 {
		return ""
	}
	eighths := int(math.Round(v / maxv * float64(width) * 8))
	full := min(width, eighths/8)
	s := strings.Repeat("█", full)
	if r := eighths % 8; r > 0 && full < width {
		s += string([]rune("▏▎▍▌▋▊▉")[r-1])
	}
	return s
}
