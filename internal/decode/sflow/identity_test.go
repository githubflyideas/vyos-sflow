package sflow

import (
	"encoding/binary"
	"testing"
	"time"
)

func words(v ...uint32) []byte {
	b := make([]byte, len(v)*4)
	for i, w := range v {
		binary.BigEndian.PutUint32(b[i*4:], w)
	}
	return b
}
func samplePacket(format uint32, body []byte) []byte {
	b := words(5, 1, 0xc0000201, 7, 1, 1000, 1, format, uint32(len(body)))
	return append(b, body...)
}
func TestExpandedSourceIdentity(t *testing.T) {
	for _, tc := range []struct{ typ, idx uint32 }{{0, 0x12345678}, {0x1234, 0xfedcba98}} {
		b := samplePacket(3, words(1, tc.typ, tc.idx, 100, 1000, 0, 0, 1, 0, 2, 0))
		d, err := Decode(b, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		want := uint64(tc.typ)<<32 | uint64(tc.idx)
		if got := uint64(d.Flows[0].SourceID); got != want {
			t.Errorf("source = %#x, want %#x", got, want)
		}
		if got := uint64(d.Flows[0].Record.Observation); got != want {
			t.Errorf("observation = %#x, want %#x", got, want)
		}
	}
}
func TestCompactAndExpandedSourceIdentityMatch(t *testing.T) {
	a, err := Decode(samplePacket(1, words(1, 2<<24|123, 100, 1000, 0, 1, 2, 0)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Decode(samplePacket(3, words(1, 2, 123, 100, 1000, 0, 0, 1, 0, 2, 0)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if uint64(a.Flows[0].SourceID) != uint64(2)<<32|123 || a.Flows[0].SourceID != b.Flows[0].SourceID {
		t.Fatal("compact and expanded identities must use type<<32|index")
	}
}
