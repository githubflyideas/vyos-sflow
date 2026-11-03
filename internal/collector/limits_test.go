package collector

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/encode"
	"net/netip"
	"testing"
	"time"
)

func TestSourceStateHardCap(t *testing.T) {
	c := New(&countSink{})
	a := netip.MustParseAddr("192.0.2.1")
	now := time.Now()
	for i := uint64(0); i < 65536; i++ {
		c.sf[sfKey{a, 0, i}] = &sfState{}
	}
	b := encode.SFlow(a, 0, 1, 0, []encode.SFlowSample{{SourceID: 65536, Rate: 100}}, nil)
	c.Handle(b, a, now)
	if got := c.StateLimited.Load(); got != 1 {
		t.Fatalf("state-limit counter = %d, want 1", got)
	}
	if len(c.sf) > 65536 {
		t.Fatalf("source state grew beyond cap: %d", len(c.sf))
	}
	// Existing sources remain usable at the cap.
	b = encode.SFlow(a, 0, 2, 0, []encode.SFlowSample{{SourceID: 1, Rate: 123}}, nil)
	c.Handle(b, a, now)
	if c.sf[sfKey{a, 0, 1}].rate != 123 {
		t.Fatal("existing source stopped updating at cap")
	}
	c.Tick(now.Add(25 * time.Hour))
	if len(c.sf) != 0 || len(c.sfAgents) != 0 {
		t.Fatal("idle state not expired")
	}
}
func TestSubAgentStateHardCap(t *testing.T) {
	c := New(&countSink{})
	a := netip.MustParseAddr("192.0.2.1")
	now := time.Now()
	for i := uint32(0); i < 65; i++ {
		c.Handle(encode.SFlow(a, i, 1, 0, nil, nil), a, now)
	}
	if got := c.StateLimited.Load(); got != 1 {
		t.Fatalf("state-limit counter = %d, want 1", got)
	}
	if got := len(c.sfAgents[a].lastSeq); got > 64 {
		t.Fatalf("subagent state grew beyond cap: %d", got)
	}
	if !c.Handle(encode.SFlow(a, 0, 2, 0, nil, nil), a, now) {
		t.Fatal("existing subagent rejected")
	}
}
