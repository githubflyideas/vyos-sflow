package collector

import (
	"math"
	"math/rand/v2"
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/encode"
	"github.com/githubflyideas/vyos-sflow/internal/flow"
)

type checkSink struct{}

func (checkSink) Submit([]flow.Record)             {}
func (checkSink) SubmitCounters([]flow.IfCounters) {}

// Two ports, traffic in through 1 and out through 2, sFlow enabled on 1
// only, sampled 1 in 100, counters of both every 20 s: only port 1 is
// listed, and its samples add up to what the device counted.
func TestIfaceChecksAgainstCounters(t *testing.T) {
	c := New(&checkSink{})
	agent := netip.MustParseAddr("192.0.2.9")
	rng := rand.New(rand.NewPCG(1, 2))
	at := time.Unix(1700000000, 0)
	var inOct, inPkt [3]uint64
	var outOct, outPkt [3]uint64
	var pool, seq, dseq uint32 = 1000, 10, 0
	var batch []encode.SFlowSample
	send := func(ctrs []encode.SFlowCounter) {
		dseq++
		c.Handle(encode.SFlow(agent, 0, dseq, dseq*10, batch, ctrs), agent, at)
		batch = batch[:0]
	}
	next := at.Add(20 * time.Second)
	for i := 0; i < 400_000; i++ {
		at = at.Add(time.Millisecond)
		size := 64 + rng.IntN(1450)
		inOct[1] += uint64(size)
		inPkt[1]++
		outOct[2] += uint64(size)
		outPkt[2]++
		pool++
		if rng.IntN(100) == 0 {
			seq++
			h, _ := encode.Frame(netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("10.0.0.5"), 443, 50000, 6, 0, size-18, 0x10)
			batch = append(batch, encode.SFlowSample{Seq: seq, SourceID: 1, Rate: 100, Pool: pool, In: 1, Out: 2, FrameLen: uint32(size), Header: h})
			if len(batch) == 7 {
				send(nil)
			}
		}
		if at.After(next) {
			next = at.Add(20 * time.Second)
			send([]encode.SFlowCounter{
				{Seq: uint32(i), IfIndex: 1, Speed: 1e10, InOctets: inOct[1], OutOctets: outOct[1], InPkts: uint32(inPkt[1]), OutPkts: uint32(outPkt[1])},
				{Seq: uint32(i), IfIndex: 2, Speed: 1e10, InOctets: inOct[2], OutOctets: outOct[2], InPkts: uint32(inPkt[2]), OutPkts: uint32(outPkt[2])},
			})
		}
	}
	got := c.IfaceChecks(at)
	if len(got) != 1 || got[0].IfIndex != 1 {
		t.Fatalf("want only port 1, where sFlow is enabled: %+v", got)
	}
	ic := got[0]
	if dev, est := ic.DevInBps, ic.EstInBps; dev <= 0 || math.Abs(est/dev-1) > 0.05 {
		t.Errorf("estimated %.0f bps, device counted %.0f bps over %.0f s", est, dev, ic.Seconds)
	}
}

// One port sampled both ways, as an H3C switch does it: an ingress and an
// egress sampler, their samples numbered in one shared series, their
// sample_pool growing by more than the rate, and the datagram numbers
// skipping as well. Every datagram arrives. The
// samples must add up to what the device counted on the port, in and out,
// and the rate must stay the declared one, with nothing reported lost.
func TestH3CStyleSamplersAddUpToCounters(t *testing.T) {
	for _, rate := range []uint32{1024, 10240} {
		c := New(&checkSink{})
		agent := netip.MustParseAddr("192.168.108.125")
		rng := rand.New(rand.NewPCG(3, uint64(rate)))
		at := time.Unix(1700000000, 0)
		var inOct, outOct uint64
		var inPkt, outPkt uint32
		var sampleSeq, poolIn, poolOut, dseq uint32
		var batch []encode.SFlowSample
		send := func(ctrs []encode.SFlowCounter) {
			dseq += 2 // the datagram numbers run ahead too, with nothing lost
			c.Handle(encode.SFlow(agent, 0, dseq, dseq*10, batch, ctrs), agent, at)
			batch = batch[:0]
		}
		const port = 2204
		ingress, egress := uint32(port), uint32(1<<24|port)
		next := at.Add(20 * time.Second)
		for i := 0; i < 3_000_000; i++ {
			at = at.Add(100 * time.Microsecond)
			size := 64 + rng.IntN(1450)
			out := rng.IntN(3) == 0 // a third of the packets leave through the port
			if out {
				outOct += uint64(size)
				outPkt++
				poolOut += 1 + uint32(rng.IntN(5)/4) // the pool counts more than it samples from
			} else {
				inOct += uint64(size)
				inPkt++
				poolIn += 1 + uint32(rng.IntN(5)/4)
			}
			if rng.IntN(int(rate)) == 0 {
				sampleSeq++ // one series for both samplers
				h, _ := encode.Frame(netip.MustParseAddr("203.0.113.5"), netip.MustParseAddr("10.0.0.5"), 443, 50000, 6, 0, size-18, 0x10)
				smp := encode.SFlowSample{Seq: sampleSeq, SourceID: ingress, Rate: rate, Pool: poolIn, In: port, Out: 7, FrameLen: uint32(size), Header: h}
				if out {
					smp.SourceID, smp.Pool, smp.In, smp.Out = egress, poolOut, 7, port
				}
				batch = append(batch, smp)
				if len(batch) == 7 {
					send(nil)
				}
			}
			if at.After(next) {
				next = at.Add(20 * time.Second)
				send([]encode.SFlowCounter{{Seq: uint32(i), IfIndex: port, Speed: 1e10, InOctets: inOct, OutOctets: outOct, InPkts: inPkt, OutPkts: outPkt}})
			}
		}
		got := c.IfaceChecks(at)
		if len(got) != 1 || got[0].IfIndex != port {
			t.Fatalf("1:%d: want only port %d listed: %+v", rate, port, got)
		}
		ic := got[0]
		for _, d := range []struct {
			name     string
			est, dev float64
		}{{"in", ic.EstInBps, ic.DevInBps}, {"out", ic.EstOutBps, ic.DevOutBps}} {
			if d.dev <= 0 || math.Abs(d.est/d.dev-1) > 0.08 {
				t.Errorf("1:%d %s: samples estimate %.0f bps, device counted %.0f bps (%.0f%%)", rate, d.name, d.est, d.dev, d.est/d.dev*100)
			}
		}
		src := c.Sources()
		if len(src) != 1 {
			t.Fatalf("sources: %+v", src)
		}
		if e := src[0].Effective; e != float64(rate) {
			t.Errorf("1:%d: effective rate %v, want the declared %d", rate, e, rate)
		}
	}
}
