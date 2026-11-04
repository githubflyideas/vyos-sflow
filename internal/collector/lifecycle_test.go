package collector

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
	"net/netip"
	"testing"
	"time"
)

type lifecycleSink struct{ records []flow.Record }

func (s *lifecycleSink) Submit(r []flow.Record)         { s.records = append(s.records, r...) }
func (*lifecycleSink) SubmitCounters([]flow.IfCounters) {}
func sendLifecycle(t *testing.T, c *Collector, sub, seq, pool, uptime uint32, src string, at time.Time) {
	t.Helper()
	a := netip.MustParseAddr("192.0.2.1")
	h, size := encode.Frame(netip.MustParseAddr(src), netip.MustParseAddr("198.51.100.1"), 1234, 443, 6, 0, 100, 0x10)
	b := encode.SFlow(a, sub, seq, uptime, []encode.SFlowSample{{Seq: seq, SourceID: 1, Rate: 100, Pool: pool, Header: h, FrameLen: size}}, nil)
	if !c.Handle(b, a, at) {
		t.Fatal("valid fixture rejected")
	}
}
func TestRestartIsolatesReservoirAndEstimator(t *testing.T) {
	for _, base := range []uint32{0, 50000} {
		sink := &lifecycleSink{}
		c := New(sink)
		for i := uint32(1); i <= 11; i++ {
			sendLifecycle(t, c, 0, base+i, i*100, 100000+i*1000, "10.0.0.1", samplingEpoch.Add(time.Duration(i)*time.Second))
		}
		// Another native sub-agent must not be reset by this one's restart.
		sendLifecycle(t, c, 7, 11, 1100, 111000, "10.0.0.7", samplingEpoch.Add(11*time.Second))
		boundary := samplingEpoch.Add(time.Hour)
		sendLifecycle(t, c, 0, 1, 100, 1000, "10.0.0.2", boundary)
		st := c.sf[sfKey{netip.MustParseAddr("192.0.2.1"), 0, 1}]
		if st.poolOK || st.checks != 0 || st.winRecv != 0 || len(st.recent) != 1 {
			t.Errorf("restart base=%d retained estimator/reservoir: poolOK=%v checks=%d window=%g recent=%d", base, st.poolOK, st.checks, st.winRecv, len(st.recent))
		}
		sink.records = nil
		sendLifecycle(t, c, 0, 3, 300, 3000, "10.0.0.2", boundary.Add(2*time.Second))
		for _, r := range sink.records {
			if r.Start.Before(boundary) {
				t.Errorf("restart base=%d re-emitted old record: %+v", base, r)
			}
		}
		other := c.sf[sfKey{netip.MustParseAddr("192.0.2.1"), 7, 1}]
		if len(other.recent) != 1 || other.lastSeq != 11 {
			t.Fatal("restart cleared another sub-agent")
		}
	}
}
func TestLongUptimeRestartIsolatesReservoir(t *testing.T) {
	sink := &lifecycleSink{}
	c := New(sink)
	for i := uint32(1); i <= 11; i++ {
		sendLifecycle(t, c, 0, 50000+i, i*100, uint32(30*24*time.Hour/time.Millisecond)+i*1000, "10.0.0.1", samplingEpoch.Add(time.Duration(i)*time.Second))
	}
	boundary := samplingEpoch.Add(time.Hour)
	sendLifecycle(t, c, 0, 1, 100, 1000, "10.0.0.2", boundary)
	st := c.sf[sfKey{netip.MustParseAddr("192.0.2.1"), 0, 1}]
	if st.poolOK || st.checks != 0 || len(st.recent) != 1 {
		t.Errorf("long-uptime restart retained calibration or reservoir: poolOK=%v checks=%d recent=%d", st.poolOK, st.checks, len(st.recent))
	}
	sink.records = nil
	sendLifecycle(t, c, 0, 3, 300, 3000, "10.0.0.2", boundary.Add(2*time.Second))
	for _, r := range sink.records {
		if r.Start.Before(boundary) {
			t.Errorf("long-uptime restart re-emitted old record at %s", r.Start)
		}
	}
}

func TestDatagramReorderKeepsHighWater(t *testing.T) {
	sink := &lifecycleSink{}
	c := New(sink)
	for _, seq := range []uint32{11, 13, 12, 14} {
		sendLifecycle(t, c, 0, seq, seq*100, seq*1000, "10.0.0.1", samplingEpoch)
	}
	ag := c.sfAgents[netip.MustParseAddr("192.0.2.1")]
	if ag.lastSeq[0] != 14 || ag.lostDgram != 1 {
		t.Fatalf("datagram highwater=%d lost=%d want 14,1", ag.lastSeq[0], ag.lostDgram)
	}
}
func TestUptimeWrapDoesNotResetEstimator(t *testing.T) {
	sink := &lifecycleSink{}
	c := New(sink)
	for i := uint32(1); i <= 14; i++ {
		sendLifecycle(t, c, 0, i, i*100, (^uint32(0)-12000)+i*1000, "10.0.0.1", samplingEpoch.Add(time.Duration(i)*time.Second))
	}
	st := c.sf[sfKey{netip.MustParseAddr("192.0.2.1"), 0, 1}]
	if !st.poolOK || len(st.recent) != 14 || st.lost != 0 {
		t.Fatalf("uptime wrap reset state: %+v", st)
	}
}
