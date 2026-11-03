package collector

import (
	"context"
	"encoding/hex"
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"net"
	"net/netip"
	"testing"
	"time"
)

// Valid protocol fixtures, not just wrong versions or listener labels.
// v5 contains one IPv4 record; v9/IPFIX contain a template and data set.
var nonSFlowFixtures = map[string]string{
	"netflow5": "00050001000003e86553f1000000000000000001000000010a000001c00002010000000000010002000000010000004000000000000003e89c4001bb001006000000000000000000",
	"netflow9": "00090002000003e86553f1000000000100000001000000100100000200080004000c00040100000c0a000001c0000201",
	"ipfix":    "000a002c6553f1000000000000000001000200100100000200080004000c00040100000c0a000001c0000201",
}

func TestRejectValidNonSFlow(t *testing.T) {
	for name, raw := range nonSFlowFixtures {
		t.Run(name, func(t *testing.T) {
			sink := &countSink{}
			c := New(sink)
			b, err := hex.DecodeString(raw)
			if err != nil {
				t.Fatal(err)
			}
			if c.Handle(b, netip.MustParseAddr("192.0.2.1"), time.Now()) {
				t.Error("accepted valid non-sFlow datagram")
			}
			c.Tick(time.Now().Add(time.Hour))
			if sink.records != 0 || sink.counters != 0 || len(c.Sources()) != 0 {
				t.Error("rejected protocol reached sink or exporter state")
			}
		})
	}
}

func TestSFlowStillAccepted(t *testing.T) {
	sink := &countSink{}
	c := New(sink)
	a := netip.MustParseAddr("192.0.2.1")
	hdr, size := encode.Frame(a, netip.MustParseAddr("192.0.2.2"), 1234, 443, 6, 0, 100, 0x10)
	b := encode.SFlow(a, 0, 1, 1000, []encode.SFlowSample{{Seq: 1, SourceID: 1, Rate: 100, Pool: 100, Header: hdr, FrameLen: size}}, nil)
	if !c.Handle(b, a, time.Now()) || sink.records != 1 {
		t.Fatal("sFlow record not delivered")
	}
}

func TestUDPRejectsValidNonSFlow(t *testing.T) {
	sink := &countSink{}
	c := New(sink)
	l, err := c.Listen("127.0.0.1:0", "sflow")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Serve(ctx, l) }()
	defer func() { cancel(); <-done }()
	conn, err := net.Dial("udp", l.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, raw := range nonSFlowFixtures {
		b, err := hex.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for l.Undecoded.Load() < uint64(len(nonSFlowFixtures)) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if got := l.Undecoded.Load(); got != uint64(len(nonSFlowFixtures)) {
		t.Fatalf("undecoded datagrams = %d", got)
	}
	if sink.records != 0 || sink.counters != 0 || len(c.Sources()) != 0 {
		t.Fatal("non-sFlow UDP reached sink or state")
	}
}
