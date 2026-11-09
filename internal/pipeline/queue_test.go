package pipeline

import (
	"github.com/githubflyideas/vyos-sflow/internal/flow"
	"testing"
)

func TestCountersCountedWithoutPersistenceQueue(t *testing.T) {
	p := New(Config{}, nil)
	for i := 0; i < 10000; i++ {
		p.SubmitCounters([]flow.IfCounters{{}, {}})
	}
	if p.CounterSamples.Load() != 20000 || p.DroppedCounters.Load() != 0 {
		t.Fatal("counter receipts lost")
	}
	p.stopping = true
	p.SubmitCounters([]flow.IfCounters{{}})
	if p.CounterSamples.Load() != 20000 || p.DroppedCounters.Load() != 1 {
		t.Fatal("shutdown admission not enforced")
	}
}
