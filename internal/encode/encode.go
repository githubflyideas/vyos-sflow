// Package encode builds synthetic sFlow v5 fixtures for tests. Frame uses
// fabricated Ethernet headers; this is not a production packet exporter.
package encode

import (
	"encoding/binary"
	"net/netip"
)

type buf struct{ b []byte }

func (w *buf) u8(v uint8)   { w.b = append(w.b, v) }
func (w *buf) u16(v uint16) { w.b = binary.BigEndian.AppendUint16(w.b, v) }
func (w *buf) u32(v uint32) { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *buf) u64(v uint64) { w.b = binary.BigEndian.AppendUint64(w.b, v) }
func (w *buf) raw(v []byte) { w.b = append(w.b, v...) }
func (w *buf) addr(a netip.Addr) {
	if a.Is4() {
		v := a.As4()
		w.raw(v[:])
	} else {
		v := a.As16()
		w.raw(v[:])
	}
}
func (w *buf) pad4() {
	for len(w.b)%4 != 0 {
		w.b = append(w.b, 0)
	}
}

// Frame builds the first bytes of an Ethernet frame carrying an IP packet of
// ipLen bytes, and returns the header bytes plus the full frame length
// including the 4-byte FCS.
func Frame(src, dst netip.Addr, sport, dport uint16, proto uint8, vlan uint16, ipLen int, tcpFlags uint8) ([]byte, uint32) {
	w := &buf{}
	w.raw([]byte{0x02, 0, 0, 0, 0, 1, 0x02, 0, 0, 0, 0, 2})
	l2 := 14
	if vlan != 0 {
		w.u16(0x8100)
		w.u16(vlan)
		l2 += 4
	}
	var l4 []byte
	switch proto {
	case 6:
		t := &buf{}
		t.u16(sport)
		t.u16(dport)
		t.u32(1)
		t.u32(0)
		t.u8(5 << 4)
		t.u8(tcpFlags)
		t.u16(65535)
		t.u32(0)
		l4 = t.b
	case 17:
		t := &buf{}
		t.u16(sport)
		t.u16(dport)
		t.u16(uint16(ipLen - 20))
		t.u16(0)
		l4 = t.b
	default:
		l4 = []byte{8, 0, 0, 0}
	}
	if src.Is4() {
		w.u16(0x0800)
		w.u8(0x45)
		w.u8(0)
		w.u16(uint16(ipLen))
		w.u32(0)
		w.u8(64)
		w.u8(proto)
		w.u16(0)
		w.addr(src)
		w.addr(dst)
	} else {
		w.u16(0x86DD)
		w.u32(6 << 28)
		w.u16(uint16(ipLen - 40))
		w.u8(proto)
		w.u8(64)
		w.addr(src)
		w.addr(dst)
	}
	w.raw(l4)
	frame := l2 + ipLen + 4
	if frame < 64 {
		frame = 64
	}
	return w.b, uint32(frame)
}

// ---------------------------------------------------------------- sFlow

type SFlowSample struct {
	Seq      uint32
	SourceID uint32
	Rate     uint32
	Pool     uint32
	Drops    uint32
	In, Out  uint32
	FrameLen uint32
	Header   []byte
}

type SFlowCounter struct {
	Seq       uint32
	IfIndex   uint32
	Speed     uint64
	InOctets  uint64
	OutOctets uint64
	InPkts    uint32
	OutPkts   uint32
}

// SFlow builds one sFlow v5 datagram.
func SFlow(agent netip.Addr, subAgent, seq, uptime uint32, flows []SFlowSample, ctrs []SFlowCounter) []byte {
	w := &buf{}
	w.u32(5)
	if agent.Is4() {
		w.u32(1)
	} else {
		w.u32(2)
	}
	w.addr(agent)
	w.u32(subAgent)
	w.u32(seq)
	w.u32(uptime)
	w.u32(uint32(len(flows) + len(ctrs)))
	for _, f := range flows {
		s := &buf{}
		s.u32(f.Seq)
		s.u32(f.SourceID)
		s.u32(f.Rate)
		s.u32(f.Pool)
		s.u32(f.Drops)
		s.u32(f.In)
		s.u32(f.Out)
		s.u32(1) // one record
		rec := &buf{}
		rec.u32(1) // ethernet
		rec.u32(f.FrameLen)
		rec.u32(4) // FCS stripped
		rec.u32(uint32(len(f.Header)))
		rec.raw(f.Header)
		rec.pad4()
		s.u32(1) // raw packet header
		s.u32(uint32(len(rec.b)))
		s.raw(rec.b)
		w.u32(1)
		w.u32(uint32(len(s.b)))
		w.raw(s.b)
	}
	for _, c := range ctrs {
		s := &buf{}
		s.u32(c.Seq)
		s.u32(c.IfIndex)
		s.u32(1)
		g := &buf{}
		g.u32(c.IfIndex)
		g.u32(6)
		g.u64(c.Speed)
		g.u32(1)
		g.u32(3)
		g.u64(c.InOctets)
		g.u32(c.InPkts)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		g.u64(c.OutOctets)
		g.u32(c.OutPkts)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		g.u32(0)
		s.u32(1)
		s.u32(uint32(len(g.b)))
		s.raw(g.b)
		w.u32(2)
		w.u32(uint32(len(s.b)))
		w.raw(s.b)
	}
	return w.b
}
