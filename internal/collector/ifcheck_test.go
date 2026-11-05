package collector

import (
	"math"
	"math/rand/v2"
	"net/netip"
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
)

type checkSink struct{}

func (checkSink) Submit([]flow.Record)             {}
func (checkSink) SubmitCounters([]flow.IfCounters) {}

// Two ports, traffic in through 1 and out through 2, sampled 1 in 100 on
// the way in, counters every 20 s: the samples add up to what the device
// counted, per port and direction.
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
	if len(got) != 2 {
		t.Fatalf("interfaces %+v", got)
	}
	for _, ic := range got {
		dev, est := ic.DevInBps, ic.EstInBps
		if ic.IfIndex == 2 {
			dev, est = ic.DevOutBps, ic.EstOutBps
		}
		if dev <= 0 || math.Abs(est/dev-1) > 0.05 {
			t.Errorf("if %d: estimated %.0f bps, device counted %.0f bps over %.0f s", ic.IfIndex, est, dev, ic.Seconds)
		}
		t.Logf("if %d: estimated %.0f bps, device %.0f bps (%.1f%%) over %.0f s", ic.IfIndex, est, dev, est/dev*100, ic.Seconds)
	}
}
