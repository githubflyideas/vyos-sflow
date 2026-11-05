package collector

import (
	"net/netip"
	"sort"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/flow"
)

// Each interface's traffic as the device counts it (its counter samples,
// the same octets SNMP reads) against what the flow samples add up to on
// it, so how far the estimate is off shows on the page.

type ifKey struct {
	agent netip.Addr
	idx   uint32
}

type ifSnap struct {
	t                   time.Time
	estIn, estOut       float64
	devIn, devOut       float64
	estInPkt, estOutPkt float64
	devInPkt, devOutPkt float64
}

type ifState struct {
	cur               ifSnap // running totals; t is the last counter sample
	atCnt             ifSnap // cur as it was at the last counter sample
	lastIn, lastOut   uint64
	lastInP, lastOutP uint64
	haveCnt           bool
	speed             uint64
	snaps             []ifSnap
}

// localIf is sFlow's interface value for the device itself.
const localIf = 0x3FFFFFFF

// ifWindow is how far back the comparison reaches; ifSnapEvery how often a
// point is kept.
const (
	ifWindow    = 10 * time.Minute
	ifSnapEvery = 30 * time.Second
	ifMax       = 4096
)

func (c *Collector) ifs(k ifKey) *ifState {
	if c.ifc == nil {
		c.ifc = map[ifKey]*ifState{}
	}
	s := c.ifc[k]
	if s == nil {
		if len(c.ifc) >= ifMax {
			return nil
		}
		s = &ifState{}
		c.ifc[k] = s
	}
	return s
}

// noteFlows adds what the records stand for to their interfaces; c.mu held.
func (c *Collector) noteFlows(agent netip.Addr, recs []flow.Record) {
	for i := range recs {
		r := &recs[i]
		m := r.Mult
		if m <= 0 {
			m = 1
		}
		b := float64(r.Bytes)
		if !r.L2 {
			b += float64(r.Packets) * 18
		}
		b *= m
		p := float64(r.Packets) * m
		if r.InIf != 0 && r.InIf != localIf {
			if s := c.ifs(ifKey{agent, r.InIf}); s != nil {
				s.cur.estIn += b
				s.cur.estInPkt += p
			}
		}
		if r.OutIf != 0 && r.OutIf != localIf {
			if s := c.ifs(ifKey{agent, r.OutIf}); s != nil {
				s.cur.estOut += b
				s.cur.estOutPkt += p
			}
		}
	}
}

// noteCounters adds the octets each interface counted since its last
// counter sample; c.mu held.
func (c *Collector) noteCounters(agent netip.Addr, cs []flow.IfCounters, now time.Time) {
	for _, x := range cs {
		if x.IfIndex == 0 {
			continue
		}
		s := c.ifs(ifKey{agent, x.IfIndex})
		if s == nil {
			continue
		}
		if x.Speed > 0 {
			s.speed = x.Speed
		}
		if s.haveCnt {
			// a counter that went back (a reset, or a 32-bit one wrapped)
			// starts again from here
			if x.InOctets >= s.lastIn && x.OutOctets >= s.lastOut {
				s.cur.devIn += float64(x.InOctets - s.lastIn)
				s.cur.devOut += float64(x.OutOctets - s.lastOut)
			}
			if x.InPkts >= s.lastInP && x.OutPkts >= s.lastOutP {
				s.cur.devInPkt += float64(x.InPkts - s.lastInP)
				s.cur.devOutPkt += float64(x.OutPkts - s.lastOutP)
			}
		} else {
			// from the first counter sample on: what was estimated before
			// it has nothing to be compared with
			s.cur.estIn, s.cur.estOut, s.cur.estInPkt, s.cur.estOutPkt = 0, 0, 0, 0
		}
		s.lastIn, s.lastOut, s.lastInP, s.lastOutP, s.haveCnt = x.InOctets, x.OutOctets, x.InPkts, x.OutPkts, true
		s.cur.t = now
		s.atCnt = s.cur
		if n := len(s.snaps); n == 0 || now.Sub(s.snaps[n-1].t) >= ifSnapEvery {
			s.snaps = append(s.snaps, s.cur)
			for len(s.snaps) > 1 && now.Sub(s.snaps[1].t) >= ifWindow {
				s.snaps = s.snaps[1:]
			}
		}
	}
}

// IfaceCheck is one interface: its traffic as the device counted it and as
// the samples estimate it, per second, over Seconds.
type IfaceCheck struct {
	Exporter  string  `json:"exporter"`
	IfIndex   uint32  `json:"ifindex"`
	Speed     uint64  `json:"speed,omitempty"`
	Seconds   float64 `json:"seconds"`
	DevInBps  float64 `json:"dev_in_bps"`
	EstInBps  float64 `json:"est_in_bps"`
	DevOutBps float64 `json:"dev_out_bps"`
	EstOutBps float64 `json:"est_out_bps"`
	DevInPps  float64 `json:"dev_in_pps"`
	EstInPps  float64 `json:"est_in_pps"`
	DevOutPps float64 `json:"dev_out_pps"`
	EstOutPps float64 `json:"est_out_pps"`
}

// IfaceChecks lists the interfaces with counters and traffic in the last
// minutes, busiest first.
func (c *Collector) IfaceChecks(now time.Time) []IfaceCheck {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []IfaceCheck
	for k, s := range c.ifc {
		if !s.haveCnt || len(s.snaps) == 0 || now.Sub(s.cur.t) > 5*time.Minute {
			continue
		}
		a, b := s.snaps[0], s.atCnt
		sec := b.t.Sub(a.t).Seconds()
		if sec < 20 {
			continue
		}
		ic := IfaceCheck{Exporter: k.agent.String(), IfIndex: k.idx, Speed: s.speed, Seconds: sec,
			DevInBps: (b.devIn - a.devIn) * 8 / sec, EstInBps: (b.estIn - a.estIn) * 8 / sec,
			DevOutBps: (b.devOut - a.devOut) * 8 / sec, EstOutBps: (b.estOut - a.estOut) * 8 / sec,
			DevInPps: (b.devInPkt - a.devInPkt) / sec, EstInPps: (b.estInPkt - a.estInPkt) / sec,
			DevOutPps: (b.devOutPkt - a.devOutPkt) / sec, EstOutPps: (b.estOutPkt - a.estOutPkt) / sec}
		if ic.DevInBps+ic.DevOutBps+ic.EstInBps+ic.EstOutBps <= 0 {
			continue
		}
		out = append(out, ic)
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i].DevInBps+out[i].DevOutBps, out[j].DevInBps+out[j].DevOutBps
		if x != y {
			return x > y
		}
		return out[i].IfIndex < out[j].IfIndex
	})
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
