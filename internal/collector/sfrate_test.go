package collector

import (
	"testing"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/decode/sflow"
)

// sampler drives one source of one agent: n samples a second apart, with
// the given sample-sequence and pool steps, and the agent's datagram counts
// advanced by dgPer received and dgLost lost per sample. Then one more
// after the 30-second window so the rate is measured; it returns the rate.
type sampler struct {
	st        *sfState
	ag        *sfAgent
	seq, pool uint32
	at        time.Time
}

func newSampler() *sampler {
	now := time.Now()
	return &sampler{st: &sfState{evalAt: now}, ag: &sfAgent{}, at: now}
}

func (s *sampler) send(rate, seqStep, poolStep uint32, dgLost uint64) {
	s.seq += seqStep
	s.pool += poolStep
	s.ag.datagrams++
	s.ag.lostDgram += dgLost
	s.st.update(&sflow.FlowSample{Rate: rate, Pool: s.pool, Seq: s.seq}, s.at, s.ag)
	s.at = s.at.Add(time.Second)
}

func (s *sampler) run(rate, seqStep, poolStep uint32, n int, lostEvery int) float64 {
	for i := 0; i < n; i++ {
		var lost uint64
		if lostEvery > 0 && i%lostEvery == lostEvery-1 {
			lost = 1
		}
		s.send(rate, seqStep, poolStep, lost)
	}
	s.at = s.at.Add(31 * time.Second)
	s.send(rate, seqStep, poolStep, 0)
	return s.st.eff
}

// Whatever rate the agent declares (1 in 1024, 10240 or 65535), that is the
// rate used when nothing is lost, whatever sample_pool and the samples' own
// sequence numbers say. An H3C switch numbers the samples of its samplers
// in one series, so each sampler sees every other number missing, and its
// pool grows by more than the rate per sample; neither is lost traffic.
func TestRateIsTheDeclaredOneWhateverPoolAndSampleNumbers(t *testing.T) {
	for _, rate := range []uint32{1024, 10240, 65535} {
		if got := newSampler().run(rate, 2, rate*6/5, 40, 0); got != float64(rate) {
			t.Errorf("1:%d, sample numbers skipping every other, pool a fifth larger, no datagram lost: rate %v, want %d", rate, got, rate)
		}
	}
}

// Datagrams lost on the way raise the rate by exactly what is missing:
// one datagram in five lost, each received one stands for 5/4.
func TestRateCountsLostDatagrams(t *testing.T) {
	for _, rate := range []uint32{1024, 10240, 65535} {
		got, want := newSampler().run(rate, 1, rate, 80, 4), float64(rate)*5/4
		if got < want*0.97 || got > want*1.03 {
			t.Errorf("1:%d with one datagram in five lost: rate %v, want about %v", rate, got, want)
		}
	}
}

// Samples the agent says it dropped raise the rate too.
func TestRateCountsAgentDrops(t *testing.T) {
	s := newSampler()
	var drops uint32
	for i := 0; i < 40; i++ {
		s.seq++
		s.ag.datagrams++
		if i%4 == 0 {
			drops++ // one dropped for every four sent
		}
		s.st.update(&sflow.FlowSample{Rate: 10240, Seq: s.seq, Drops: drops}, s.at, s.ag)
		s.at = s.at.Add(time.Second)
	}
	s.at = s.at.Add(31 * time.Second)
	s.seq++
	s.ag.datagrams++
	s.st.update(&sflow.FlowSample{Rate: 10240, Seq: s.seq, Drops: drops}, s.at, s.ag)
	if want := 10240.0 * 5 / 4; s.st.eff < want*0.95 || s.st.eff > want*1.05 {
		t.Errorf("one sample in five dropped by the agent: rate %v, want about %v", s.st.eff, want)
	}
}

// Changing the rate on the switch (1:1024 to 1:65535) is followed at once,
// and nothing measured against the old rate is carried over.
func TestRateFollowsAChangeOnTheSwitch(t *testing.T) {
	s := newSampler()
	if got := s.run(1024, 1, 1024, 40, 0); got != 1024 {
		t.Fatalf("before the change: %v", got)
	}
	s.send(65535, 1, 65535, 0)
	if s.st.eff != 0 || s.st.rate != 65535 {
		t.Errorf("after the change: measured rate %v kept, declared %d", s.st.eff, s.st.rate)
	}
	if got := s.run(65535, 1, 65535, 40, 0); got != 65535 {
		t.Errorf("measured again after the change: %v, want 65535", got)
	}
}

// The same sample delivered twice is counted once.
func TestRepeatedSampleIgnored(t *testing.T) {
	s := newSampler()
	s.send(10240, 1, 10240, 0)
	if s.st.update(&sflow.FlowSample{Rate: 10240, Pool: s.pool, Seq: s.seq}, s.at, s.ag) {
		t.Error("a repeated sample was accepted")
	}
}
