package collector

import (
	"context"
	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"net"
	"testing"
	"time"
)

type nativeHeaderSink struct{ rows []flow.Record }

func (s *nativeHeaderSink) Submit(rows []flow.Record)      { s.rows = append(s.rows, rows...) }
func (*nativeHeaderSink) SubmitCounters([]flow.IfCounters) {}

// Production intentionally leaves SetAllowedStream unused. Native header IPs
// need not match the peer, and the first packet must not lock later identities.
func TestUnrestrictedRealUDPHeaders(t *testing.T) {
	sink := &nativeHeaderSink{}
	c := New(sink)
	l, err := c.Listen("0.0.0.0:0", "sflow")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Serve(ctx, l) }()
	defer func() { cancel(); <-done }()
	_, port, err := net.SplitHostPort(l.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i, agent := range []string{"192.0.2.1", "192.0.2.1", "198.51.100.2"} {
		if _, err = conn.Write(streamPacket(agent, uint32(100000+i), uint32(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n uint64
		for _, s := range c.Sources() {
			n += s.Records
		}
		if n == 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if len(sink.rows) != 3 || c.RejectedStreams.Load() != 0 {
		t.Fatalf("accepted=%d rejected=%d", len(sink.rows), c.RejectedStreams.Load())
	}
	for i, want := range []string{"192.0.2.1", "192.0.2.1", "198.51.100.2"} {
		if sink.rows[i].Exporter.String() != want {
			t.Fatalf("native header replaced by UDP peer: %+v", sink.rows[i])
		}
	}
	if s := c.StreamStatus(); s.Enforced || s.Configured || s.Agent != "" {
		t.Fatalf("unexpected auto-binding: %+v", s)
	}
	if len(c.sf) != 3 || len(c.Sources()) != 2 {
		t.Fatalf("native identities lost: states=%d sources=%+v", len(c.sf), c.Sources())
	}
}
