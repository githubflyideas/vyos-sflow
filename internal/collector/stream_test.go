package collector

import (
	"context"
	"encoding/hex"
	"github.com/githubflyideas/vyos-sflow/internal/encode"
	"net"
	"net/netip"
	"testing"
	"time"
)

// Reflection-free interface assertion lets the RED report the missing policy.
func setStream(t *testing.T, c *Collector, agent string, sub uint32) {
	t.Helper()
	policy, ok := any(c).(interface{ SetAllowedStream(string, uint32) error })
	if !ok {
		t.Fatal("collector has no explicit single-stream admission policy")
	}
	if err := policy.SetAllowedStream(agent, sub); err != nil {
		t.Fatal(err)
	}
}
func streamPacket(agent string, sub, source uint32) []byte {
	a := netip.MustParseAddr(agent)
	hdr, size := encode.Frame(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), uint16(source), 443, 6, 0, 100, 0x10)
	return encode.SFlow(a, sub, source, 1000, []encode.SFlowSample{{Seq: source, SourceID: source, Rate: 100, Pool: 100, Header: hdr, FrameLen: size}}, nil)
}
func TestStreamEdgeCases(t *testing.T) {
	for _, agent := range []string{"192.0.2.1", "::ffff:192.0.2.1", "2001:db8::1"} {
		t.Run(agent, func(t *testing.T) {
			sink := &countSink{}
			c := New(sink)
			setStream(t, c, agent, 9)
			peer := netip.MustParseAddr("127.0.0.1")
			now := time.Now()
			c.Handle(streamPacket("192.0.2.99", 9, 1), peer, now)
			c.Handle(encode.SFlow(netip.MustParseAddr(agent), 8, 1, 1000, nil, []encode.SFlowCounter{{IfIndex: 1}}), peer, now)
			good := streamPacket(agent, 9, 1)
			if c.Handle(good[:len(good)-1], peer, now) {
				t.Error("malformed packet accepted")
			}
			for _, raw := range nonSFlowFixtures {
				b, _ := hex.DecodeString(raw)
				if c.Handle(b, peer, now) {
					t.Error("foreign protocol accepted")
				}
			}
			if len(c.Sources()) != 0 || len(c.sf) != 0 || sink.records != 0 || sink.counters != 0 {
				t.Fatal("rejected packet polluted state")
			}
			if c.RejectedStreams.Load() != 2 {
				t.Fatalf("rejected=%d", c.RejectedStreams.Load())
			}
			if !c.Handle(good, peer, now) || sink.records != 1 {
				t.Fatal("valid packet refused")
			}
			if !c.Handle(encode.SFlow(netip.MustParseAddr(agent), 9, 2, 1000, nil, []encode.SFlowCounter{{IfIndex: 1}}), peer, now) || sink.counters != 1 {
				t.Fatal("valid counter refused")
			}
			status := c.StreamStatus()
			if !status.Configured || status.SubAgent != 9 || status.Rejected != 2 || status.Agent != netip.MustParseAddr(agent).Unmap().String() {
				t.Fatalf("bad status %+v", status)
			}
		})
	}
}
func TestStreamPolicyValidation(t *testing.T) {
	c := New(&countSink{})
	for _, agent := range []string{"bad", "192.0.2.1:6343", "fe80::1%eth0"} {
		if err := c.SetAllowedStream(agent, 0); err == nil {
			t.Errorf("accepted %q", agent)
		}
	}
	setStream(t, c, "", 0)
	if s := c.StreamStatus(); !s.Enforced || s.Configured || s.Message == "" {
		t.Fatalf("missing required diagnostic %+v", s)
	}
	if err := c.SetAllowedStream("192.0.2.1", 0); err == nil {
		t.Fatal("live stream policy changed")
	}
}
func TestStreamUDPPortChanges(t *testing.T) {
	sink := &countSink{}
	c := New(sink)
	setStream(t, c, "192.0.2.1", 0)
	l, err := c.Listen("127.0.0.1:0", "sflow")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Serve(ctx, l) }()
	defer func() { cancel(); <-done }()
	// Keep both sockets open to prove distinct peer ports, same header identity.
	for _, source := range []uint32{1, 2} {
		conn, err := net.Dial("udp", l.LocalAddr())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err = conn.Write(streamPacket("192.0.2.1", 0, source)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ss := c.Sources()
		if len(ss) == 1 && ss[0].Records == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if sink.records != 2 || c.RejectedStreams.Load() != 0 {
		t.Fatalf("UDP acceptance records=%d", sink.records)
	}
}
func TestExplicitSingleStream(t *testing.T) {
	sink := &countSink{}
	c := New(sink)
	setStream(t, c, "::ffff:192.0.2.1", 7)
	peer := netip.MustParseAddr("127.0.0.1")
	for _, b := range [][]byte{streamPacket("192.0.2.2", 7, 1), streamPacket("192.0.2.1", 8, 1)} {
		c.Handle(b, peer, time.Now())
	}
	if sink.records != 0 || len(c.Sources()) != 0 || len(c.sf) != 0 {
		t.Fatal("foreign streams polluted sink/state")
	}
	for _, source := range []uint32{1, 2} {
		if !c.Handle(streamPacket("192.0.2.1", 7, source), peer, time.Now()) {
			t.Fatal("configured stream rejected")
		}
	}
	if c.Sources()[0].Domain != 7 {
		t.Fatalf("wrong sub-agent diagnostic: %+v", c.Sources())
	}
	if sink.records != 2 || len(c.sf) != 2 || len(c.Sources()) != 1 {
		t.Fatalf("multi-interface stream lost: %+v", c.Sources())
	}
}
func TestUnconfiguredStreamFailsClosed(t *testing.T) {
	for i := 0; i < 2; i++ { // fresh/restarted collectors must not bind the first packet
		sink := &countSink{}
		c := New(sink)
		setStream(t, c, "", 0)
		c.Handle(streamPacket("192.0.2.1", 0, 1), netip.MustParseAddr("127.0.0.1"), time.Now())
		if sink.records != 0 || len(c.Sources()) != 0 || len(c.sf) != 0 {
			t.Fatal("unconfigured collector accepted first packet")
		}
	}
}
