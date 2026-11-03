package collector

import (
	"encoding/binary"
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"net/netip"
	"testing"
	"time"
)

func TestMalformedSFlowDoesNotPublishPartialDatagram(t *testing.T) {
	a := netip.MustParseAddr("192.0.2.1")
	hdr, size := encode.Frame(a, netip.MustParseAddr("192.0.2.2"), 1234, 443, 6, 0, 100, 0x10)
	b := encode.SFlow(a, 0, 1, 1000, []encode.SFlowSample{{Seq: 1, SourceID: 1, Rate: 100, Pool: 100, Header: hdr, FrameLen: size}}, nil)
	// One valid flow followed by a counter sample missing its mandatory header.
	binary.BigEndian.PutUint32(b[24:], 2)
	b = append(b, 0, 0, 0, 2, 0, 0, 0, 0)
	sink := &countSink{}
	c := New(sink)
	if c.Handle(b, a, time.Now()) {
		t.Error("malformed sFlow reported understood")
	}
	if sink.records != 0 || sink.counters != 0 {
		t.Fatal("partial datagram reached the sink")
	}
	if ss := c.Sources(); len(ss) != 1 || ss[0].DecodeErrors != 1 {
		t.Fatalf("decode error not observable: %+v", ss)
	}
}
