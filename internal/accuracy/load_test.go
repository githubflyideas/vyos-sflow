//go:build linux || darwin

package accuracy

import (
	"context"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/collector"
	"github.com/githubflyideas/vyos-sflow/internal/encode"
	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"github.com/githubflyideas/vyos-sflow/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow/internal/store"
)

// loadSetup applies the program's memory rules (database share and the same
// soft limit for Go, each at least 256 MB) and skips machines too small for
// a load test, so an undersized runner is not mistaken for a product failure.
func loadSetup(t *testing.T) float64 {
	if os.Getenv("T66_LOAD") == "" {
		t.Skip("set T66_LOAD=1 to run the load test")
	}
	total := store.TotalMemory()
	if total < 1500<<20 {
		t.Skipf("load test needs at least 1.5 GB of memory; this machine has %d MB", total>>20)
	}
	frac := 0.10
	if v, err := strconv.ParseFloat(os.Getenv("T66_LOAD_MEMORY"), 64); err == nil {
		frac = v
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		lim := int64(float64(total) * frac)
		if lim < 256<<20 {
			lim = 256 << 20
		}
		debug.SetMemoryLimit(lim)
	}
	return frac
}

func cpuTime() time.Duration {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// TestLoadUDP sends sFlow over UDP at the design rate and checks that
// nothing is dropped. Run with T66_LOAD=1 (optionally T66_LOAD_RATE, _SECS).
func TestLoadUDP(t *testing.T) {
	frac := loadSetup(t)
	rate, _ := strconv.Atoi(os.Getenv("T66_LOAD_RATE"))
	if rate == 0 {
		rate = 5000
	}
	secs, _ := strconv.Atoi(os.Getenv("T66_LOAD_SECS"))
	if secs == 0 {
		secs = 60
	}
	st, err := store.Open(store.Options{Dir: t.TempDir(), RawDays: 30, MemoryFraction: frac})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := pipeline.New(pipeline.Config{}, st)
	c := collector.New(p)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); <-p.Done() }()
	go p.Run(ctx)
	l, err := c.Listen("127.0.0.1:0", "sflow")
	if err != nil {
		t.Fatal(err)
	}
	go c.Serve(ctx, l)
	conn, err := net.Dial("udp", l.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rng := rand.New(rand.NewPCG(7, 8))
	const perMsg = 30
	msgs := rate / perMsg
	cpu0 := cpuTime()
	start := time.Now()
	var seq uint32
	sent := 0
	tick := time.NewTicker(time.Second / time.Duration(msgs))
	defer tick.Stop()
	for i := 0; i < msgs*secs; i++ {
		<-tick.C
		var samples []encode.SFlowSample
		for j := 0; j < perMsg; j++ {
			cl := netip.AddrFrom4([4]byte{10, 1, byte(rng.IntN(20)), byte(rng.IntN(250))})
			sv := netip.AddrFrom4([4]byte{192, 0, 2, byte(1 + rng.IntN(250))})
			hdr, size := encode.Frame(cl, sv, uint16(1024+rng.IntN(60000)), 443, 6, 0, 100+rng.IntN(1400), 0x10)
			seq++
			samples = append(samples, encode.SFlowSample{Seq: seq, SourceID: 1, Rate: 1, Pool: seq, In: 1, Out: 2, FrameLen: size, Header: hdr})
		}
		if _, err := conn.Write(encode.SFlow(netip.MustParseAddr("192.0.2.254"), 0, uint32(i+1), uint32(time.Since(start).Milliseconds()), samples, nil)); err != nil {
			t.Fatal(err)
		}
		sent += perMsg
	}
	time.Sleep(3 * time.Second)
	el := time.Since(start)
	cancel()
	<-p.Done()
	cpu := cpuTime() - cpu0
	got := p.Records.Load()
	t.Logf("sent %d records in %s (%.0f/s); pipeline saw %d, dropped %d, undecoded datagrams %d, lost per sequence %d",
		sent, el.Round(time.Second), float64(sent)/el.Seconds(), got, p.Dropped.Load(), l.Undecoded.Load(), c.Sources()[0].LostRecords)
	t.Logf("CPU %.0f%% of one core (including the sender)", cpu.Seconds()/el.Seconds()*100)
	logPeakMemory(t)
	if p.Dropped.Load() > 0 || got < uint64(sent) {
		t.Errorf("records were lost: sent %d, processed %d, dropped %d", sent, got, p.Dropped.Load())
	}
}

// TestLoadQueries bulk-loads hours of flows at the design rate and times
// the queries the UI makes. Run with T66_LOAD=1 T66_LOAD_HOURS=n
// (T66_LOAD_MEMORY sets the database's memory share, GOMEMLIMIT the Go one).
func TestLoadQueries(t *testing.T) {
	frac := loadSetup(t)
	hours, _ := strconv.Atoi(os.Getenv("T66_LOAD_HOURS"))
	if hours == 0 {
		hours = 1
	}
	dir := os.Getenv("T66_LOAD_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	st, err := store.Open(store.Options{Dir: dir, RawDays: 30, MemoryFraction: frac})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := pipeline.New(pipeline.Config{}, st)
	rng := rand.New(rand.NewPCG(9, 10))
	end := time.Now().UTC().Truncate(time.Hour)
	begin := end.Add(-time.Duration(hours) * time.Hour)
	exp := netip.MustParseAddr("10.0.0.1")
	servers := make([]netip.Addr, 50000)
	for i := range servers {
		servers[i] = netip.AddrFrom4([4]byte{byte(1 + rng.IntN(220)), byte(rng.IntN(256)), byte(rng.IntN(256)), byte(1 + rng.IntN(250))})
	}
	spoofed := os.Getenv("T66_LOAD_SPOOFED") != "" // every remote address different, as in a spoofed flood
	// Each client talks to its own ~100 servers per hour, drawn from the
	// popular ones, as on real networks. T66_LOAD_UNIQUE=1 makes every flow a
	// new conversation (worst case).
	unique := os.Getenv("T66_LOAD_UNIQUE") != ""
	peersOf := map[netip.Addr][]netip.Addr{}
	t0 := time.Now()
	var n int
	for m := begin; m.Before(end); m = m.Add(time.Minute) {
		// one minute at the design rate, written in chunks as the live
		// pipeline does, so the test itself fits on small machines
		const chunk = 20000
		recs := make([]flow.Record, 0, chunk)
		for i := 0; i < 5000*60; i++ {
			ts := m.Add(time.Duration(rng.IntN(60000)) * time.Millisecond)
			cl := netip.AddrFrom4([4]byte{10, 1, byte(rng.IntN(20)), byte(rng.IntN(250))})
			// popular servers get most flows, like real traffic
			sv := servers[int(float64(len(servers))*rng.Float64()*rng.Float64()*rng.Float64())]
			if !unique && !spoofed {
				ps := peersOf[cl]
				if len(ps) < 100 {
					peersOf[cl] = append(ps, sv)
				} else {
					sv = ps[rng.IntN(len(ps))]
				}
			}
			if spoofed {
				sv = netip.AddrFrom4([4]byte{byte(1 + rng.IntN(220)), byte(rng.IntN(256)), byte(rng.IntN(256)), byte(1 + rng.IntN(250))})
			}
			pk := uint64(1 + rng.IntN(30))
			recs = append(recs, flow.Record{Start: ts.Add(-time.Duration(rng.IntN(30)) * time.Second), End: ts, Src: cl, Dst: sv,
				SrcPort: uint16(1024 + rng.IntN(60000)), DstPort: []uint16{443, 80, 53, 22, 8801}[rng.IntN(5)], Proto: 6,
				Bytes: pk * uint64(40+rng.IntN(1460)), Packets: pk, Mult: 100, Sampling: 100, SamplingKnown: true,
				InIf: 1, OutIf: 2, Exporter: exp, Source: flow.SrcSFlow})
			if len(recs) == chunk || i == 5000*60-1 {
				p.Ingest(recs)
				p.FlushRows()
				n += len(recs)
				recs = recs[:0]
			}
		}
		p.FlushRollups() // the live pipeline flushes rollups every minute too
		if m.Minute() == 59 {
			if err := st.Seal(m.Add(2 * time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
	}
	p.FlushRollups()
	st.Seal(end.Add(time.Hour))
	load := time.Since(t0)
	u := st.Usage()
	logPeakMemory(t)
	t.Logf("loaded %d records (%d rows) over %d h in %s = %.0f records/s; disk %.2f GB (%.1f bytes/row)",
		n, p.Rows.Load(), hours, load.Round(time.Second), float64(n)/load.Seconds(), float64(u.DiskBytes)/1e9, float64(u.DiskBytes)/float64(p.Rows.Load()))

	timeIt := func(name string, f func() error) {
		t0 := time.Now()
		if err := f(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		t.Logf("%-40s %6d ms", name, time.Since(t0).Milliseconds())
	}
	full := store.Query{From: end.Add(-24 * time.Hour), To: end}
	last1h := store.Query{From: end.Add(-time.Hour), To: end}
	host := []store.Filter{{Field: "ip", Value: "10.1.3.7"}}
	timeIt("24h totals (rollup)", func() error { _, e := st.Totals(full); return e })
	timeIt("24h series by app (rollup)", func() error { _, e := st.SeriesByApp(full, 6); return e })
	timeIt("24h top 66 clients (rollup)", func() error { _, e := st.TopN(full, "host", 66); return e })
	timeIt("24h top 66 ports (rollup)", func() error { _, e := st.TopN(full, "app", 66); return e })
	timeIt("1h totals (detail)", func() error { _, e := st.Totals(last1h); return e })
	timeIt("1h top 66 conversations (detail)", func() error { _, e := st.TopN(last1h, "host", 66); return e })
	timeIt("1h series by app (detail)", func() error { _, e := st.SeriesByApp(last1h, 6); return e })
	full.Filters, last1h.Filters = host, host
	timeIt("1h one host: totals (detail)", func() error { _, e := st.Totals(last1h); return e })
	timeIt("one host, all loaded data: top 66 servers", func() error { _, e := st.TopN(full, "host", 66); return e })
	timeIt("one host, all loaded data: series", func() error { _, e := st.SeriesByApp(full, 6); return e })
}

func logPeakMemory(t *testing.T) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("Go heap in use after GC: %d MB (Go total from the OS: %d MB)", ms.HeapInuse>>20, ms.Sys>>20)
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "VmHWM") {
				t.Logf("peak memory (resident): %s", strings.TrimSpace(strings.TrimPrefix(l, "VmHWM:")))
			}
		}
	}
}
