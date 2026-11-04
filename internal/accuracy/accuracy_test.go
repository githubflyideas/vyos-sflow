// Package accuracy holds end-to-end tests: real export datagrams go through
// the collector, the pipeline and the store, and the stored link-layer
// totals are compared with the traffic that was generated.
package accuracy

import (
	"math"
	"math/rand/v2"
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/collector"
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"github.com/githubflyideas/vyos-sflow-new/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

// syncSink feeds the pipeline synchronously so the test is deterministic.
type syncSink struct{ p *pipeline.Pipeline }

func (s syncSink) Submit(r []flow.Record)             { s.p.Ingest(r) }
func (s syncSink) SubmitCounters(c []flow.IfCounters) { s.p.SubmitCounters(c) }

func setup(t *testing.T) (*collector.Collector, *pipeline.Pipeline, *store.Store) {
	t.Helper()
	st, err := store.Open(store.Options{Dir: t.TempDir(), RawDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := pipeline.New(pipeline.Config{}, st)
	c := collector.New(syncSink{p})
	c.Live = false
	return c, p, st
}

func wireTotal(t *testing.T, p *pipeline.Pipeline, st *store.Store, exporter string, from, to time.Time) float64 {
	t.Helper()
	p.FlushRows()
	p.FlushRollups()
	tot, err := st.Totals(store.Query{From: from, To: to, Filters: []store.Filter{{Field: "exporter", Value: exporter}}})
	if err != nil {
		t.Fatal(err)
	}
	return float64(tot.Wire)
}

func check(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	e := (got - want) / want
	t.Logf("%s: stored %.0f bytes, generated %.0f bytes, error %+.2f%%", what, got, want, e*100)
	if math.Abs(e) > tol {
		t.Errorf("%s: error %+.2f%% exceeds ±%.1f%%", what, e*100, tol*100)
	}
}

var (
	clients = []netip.Addr{netip.MustParseAddr("10.1.0.10"), netip.MustParseAddr("10.1.0.11"), netip.MustParseAddr("10.1.0.12")}
	servers = []netip.Addr{netip.MustParseAddr("198.51.100.20"), netip.MustParseAddr("203.0.113.30")}
)

// sFlow, 1:1000, random frame sizes, 3% of datagrams lost on the way.
func TestSFlowSampledWithDatagramLoss(t *testing.T) {
	c, p, st := setup(t)
	agent := netip.MustParseAddr("10.0.0.2")
	rng := rand.New(rand.NewPCG(1, 2))
	const rate = 1000
	start := time.Now().UTC().Truncate(time.Minute).Add(-20 * time.Minute)
	var truth float64
	var pool, seq, dseq uint32
	var batch []encode.SFlowSample
	now := start
	send := func() {
		dseq++
		if rng.Float64() >= 0.03 {
			c.Handle(encode.SFlow(agent, 0, dseq, 1000, batch, nil), agent, now)
		}
		batch = batch[:0]
	}
	const packets = 60_000_000 // ten minutes at 100 kpps
	for i := 0; i < packets; i++ {
		ip := 46 + rng.IntN(1500-46+1)
		truth += float64(ip + 18)
		pool++
		if i%rate != rate-1 {
			continue
		}
		now = start.Add(time.Duration(float64(i) / packets * float64(10*time.Minute)))
		seq++
		cl, sv := clients[rng.IntN(len(clients))], servers[rng.IntN(len(servers))]
		hdr, _ := encode.Frame(cl, sv, 40000, 443, 6, 0, ip, 0x18)
		batch = append(batch, encode.SFlowSample{Seq: seq, SourceID: 5, Rate: rate, Pool: pool, In: 5, Out: 6, FrameLen: uint32(ip + 18), Header: hdr})
		if len(batch) == 8 {
			send()
		}
	}
	if len(batch) > 0 {
		send()
	}
	got := wireTotal(t, p, st, agent.String(), start.Add(-time.Hour), start.Add(time.Hour))
	check(t, "sFlow 1:1000 with 3% datagram loss", got, truth, 0.01)
}

// sFlow, 1:1, a one-second burst in which the receiver loses runs of
// datagrams: the samples that arrive make up for those that did not.
func TestSFlowBurstWithRunsLost(t *testing.T) {
	c, p, st := setup(t)
	agent := netip.MustParseAddr("10.0.0.3")
	rng := rand.New(rand.NewPCG(3, 4))
	start := time.Now().UTC().Truncate(time.Minute).Add(-20 * time.Minute)
	var truth float64
	var pool, seq, dseq uint32
	var batch []encode.SFlowSample
	now := start
	lose := 0
	send := func() {
		dseq++
		if lose == 0 && rng.Float64() < 0.01 {
			lose = 10 + rng.IntN(300) // a run of datagrams lost together
		}
		if lose > 0 {
			lose--
		} else {
			c.Handle(encode.SFlow(agent, 0, dseq, 1000, batch, nil), agent, now)
		}
		batch = batch[:0]
	}
	// a quiet minute first, then 15,000 packets in a second, then quiet
	for i := 0; i < 16_200; i++ {
		ip := 1500
		switch {
		case i < 600:
			now, ip = start.Add(time.Duration(i)*100*time.Millisecond), 80
		case i < 15_600:
			now = start.Add(time.Minute + time.Duration(i-600)*time.Second/15_000)
		default:
			now, ip = start.Add(2*time.Minute+time.Duration(i-15_600)*100*time.Millisecond), 80
		}
		truth += float64(ip + 18)
		pool++
		seq++
		hdr, _ := encode.Frame(clients[0], servers[0], 40000, 443, 6, 0, ip, 0x18)
		batch = append(batch, encode.SFlowSample{Seq: seq, SourceID: 5, Rate: 1, Pool: pool, In: 5, Out: 6, FrameLen: uint32(ip + 18), Header: hdr})
		if len(batch) == 7 {
			send()
		}
	}
	if len(batch) > 0 {
		send()
	}
	got := wireTotal(t, p, st, agent.String(), start.Add(-time.Hour), start.Add(time.Hour))
	check(t, "sFlow 1:1 burst with runs of datagrams lost", got, truth, 0.01)
}

// sFlow, 1:1, a burst of full-size packets whose second half the receiver
// loses in one run, up to its end; the first sample to arrive after it is
// a small packet. The lost packets count as the burst they were part of.
func TestSFlowBurstTailLost(t *testing.T) {
	c, p, st := setup(t)
	agent := netip.MustParseAddr("10.0.0.4")
	start := time.Now().UTC().Truncate(time.Minute).Add(-20 * time.Minute)
	var truth float64
	var pool, seq, dseq uint32
	var batch []encode.SFlowSample
	now := start
	const burst = 15_000
	send := func(lost bool) {
		dseq++
		if !lost {
			c.Handle(encode.SFlow(agent, 0, dseq, 1000, batch, nil), agent, now)
		}
		batch = batch[:0]
	}
	sample := func(ip int, lost bool) {
		truth += float64(ip + 18)
		pool++
		seq++
		hdr, _ := encode.Frame(clients[0], servers[0], 40000, 443, 6, 0, ip, 0x18)
		batch = append(batch, encode.SFlowSample{Seq: seq, SourceID: 5, Rate: 1, Pool: pool, In: 5, Out: 6, FrameLen: uint32(ip + 18), Header: hdr})
		if len(batch) == 6 {
			send(lost)
		}
	}
	for i := 0; i < 300; i++ { // quiet before
		now = start.Add(time.Duration(i) * 200 * time.Millisecond)
		sample(80, false)
	}
	for i := 0; i < burst; i++ {
		now = start.Add(time.Minute + time.Duration(i)*time.Second/burst)
		ip := 1500
		if i%15 == 14 {
			ip = 52 // an ack now and then
		}
		sample(ip, i > burst/2)
	}
	for i := 0; i < 300; i++ { // quiet after
		now = start.Add(2*time.Minute + time.Duration(i)*200*time.Millisecond)
		sample(80, false)
	}
	got := wireTotal(t, p, st, agent.String(), start.Add(-time.Hour), start.Add(time.Hour))
	check(t, "sFlow 1:1 burst with its tail lost", got, truth, 0.02)
}

// sFlow, 1:100, two samplers under one source id (one per direction), each
// with its own pool and sequence number, the receiver losing runs of
// datagrams: both count in full.
func TestSFlowTwoSamplersOneSource(t *testing.T) {
	c, p, st := setup(t)
	agent := netip.MustParseAddr("10.0.0.5")
	rng := rand.New(rand.NewPCG(5, 6))
	start := time.Now().UTC().Truncate(time.Minute).Add(-20 * time.Minute)
	var truth float64
	pool := [2]uint32{1_000_000, 70_000}
	seq := [2]uint32{10_000, 700}
	var dseq uint32
	var batch []encode.SFlowSample
	now := start
	lose := 0
	send := func() {
		dseq++
		if lose == 0 && rng.Float64() < 0.02 {
			lose = 1 + rng.IntN(20)
		}
		if lose > 0 {
			lose--
		} else {
			c.Handle(encode.SFlow(agent, 0, dseq, 1000, batch, nil), agent, now)
		}
		batch = batch[:0]
	}
	for i := 0; i < 20_000; i++ {
		now = start.Add(time.Duration(i) * 5 * time.Millisecond)
		d := rng.IntN(2) // either direction, in no order
		ip := 40 + rng.IntN(1460)
		step := uint32(1 + rng.IntN(199)) // packets per sample, 100 on average
		pool[d] += step
		seq[d]++
		truth += float64(step) * float64(ip+18)
		hdr, _ := encode.Frame(clients[d], servers[d], 40000, 443, 6, 0, ip, 0x18)
		batch = append(batch, encode.SFlowSample{Seq: seq[d], SourceID: 5, Rate: 100, Pool: pool[d], In: 5, Out: 6, FrameLen: uint32(ip + 18), Header: hdr})
		if len(batch) == 7 {
			send()
		}
	}
	if len(batch) > 0 {
		send()
	}
	got := wireTotal(t, p, st, agent.String(), start.Add(-time.Hour), start.Add(time.Hour))
	check(t, "sFlow 1:100, two samplers under one source id", got, truth, 0.02)
}
