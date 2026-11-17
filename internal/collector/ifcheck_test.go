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
