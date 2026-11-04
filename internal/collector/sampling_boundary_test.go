package collector

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/decode/sflow"
	"testing"
	"time"
)

var samplingEpoch = time.Unix(1700000000, 0)

func warmSampling(rate uint32) *sfState {
	st := &sfState{evalAt: samplingEpoch}
	for i := uint32(1); i <= 11; i++ {
		st.update(&sflow.FlowSample{Rate: rate, Pool: i * rate, Seq: i}, samplingEpoch.Add(time.Duration(i)*time.Second))
	}
	return st
}

func TestReorderPreservesSamplingHighWater(t *testing.T) {
	st := warmSampling(100)
	for _, x := range []struct {
		seq, pool uint32
		want      float64
	}{{13, 1300, 200}, {12, 1200, 0}, {14, 1400, 100}} {
		w, _ := st.update(&sflow.FlowSample{Rate: 100, Pool: x.pool, Seq: x.seq, Drops: 2}, samplingEpoch)
		if w != x.want {
			t.Errorf("seq=%d weight=%g want=%g", x.seq, w, x.want)
		}
		if x.seq == 12 && (st.lastSeq != 13 || st.lastPool != 1300) {
			t.Errorf("late sample regressed high water: seq=%d pool=%d", st.lastSeq, st.lastPool)
		}
	}
	if st.lost != 1 {
		t.Errorf("lost=%d want 1, not an invented second gap", st.lost)
	}
	if st.winRecv != 12 {
		t.Errorf("window=%g want 12 forward observations", st.winRecv)
	}
}

func TestLateOldRatePreservesCurrentRate(t *testing.T) {
	st := warmSampling(1000)
	st.update(&sflow.FlowSample{Rate: 100, Pool: 11100, Seq: 12}, samplingEpoch)
	w, _ := st.update(&sflow.FlowSample{Rate: 1000, Pool: 11000, Seq: 11}, samplingEpoch)
	if w != 0 || st.rate != 100 || st.winPool != 100 || st.winRecv != 1 {
		t.Fatalf("late old-rate sample: weight=%g rate=%d window=%g/%g; want 0,100,100/1", w, st.rate, st.winPool, st.winRecv)
	}
}

func TestSamplingWrapControls(t *testing.T) {
	for _, mode := range []string{"pool", "sequence", "both"} {
		t.Run(mode, func(t *testing.T) {
			st := &sfState{evalAt: samplingEpoch}
			var pool, seq uint32
			if mode == "pool" || mode == "both" {
				pool = ^uint32(0) - 1150
			}
			if mode == "sequence" || mode == "both" {
				seq = ^uint32(0) - 11
			}
			for i := 0; i < 14; i++ {
				pool += 100
				seq++
				w, g := st.update(&sflow.FlowSample{Rate: 100, Pool: pool, Seq: seq}, samplingEpoch)
				if w != 100 || g != 0 {
					t.Fatalf("i=%d pool=%d seq=%d weight=%g gap=%d", i, pool, seq, w, g)
				}
			}
			if st.lost != 0 || !st.poolOK {
				t.Fatalf("wrap lost=%d poolOK=%v", st.lost, st.poolOK)
			}
		})
	}
}

func TestInOrderRateChangesRetainPoolEstimator(t *testing.T) {
	for _, rates := range [][2]uint32{{1000, 100}, {100, 1000}} {
		st := warmSampling(rates[0])
		st.eff = float64(rates[0])
		w, g := st.update(&sflow.FlowSample{Rate: rates[1], Pool: 11*rates[0] + rates[1], Seq: 12}, samplingEpoch)
		if w != float64(rates[1]) || g != 0 || st.eff != 0 || !st.poolOK {
			t.Fatalf("rate %v weight=%g gap=%d eff=%g poolOK=%v", rates, w, g, st.eff, st.poolOK)
		}
	}
}

func TestDuplicateLatestAddsNoWeight(t *testing.T) {
	for _, n := range []uint32{1, 11} {
		st := &sfState{evalAt: samplingEpoch}
		for i := uint32(1); i <= n; i++ {
			st.update(&sflow.FlowSample{Rate: 100, Pool: i * 100, Seq: i}, samplingEpoch)
		}
		beforeRecv := st.winRecv
		w, g := st.update(&sflow.FlowSample{Rate: 100, Pool: n * 100, Seq: n}, samplingEpoch)
		if w != 0 || g != 0 || st.winRecv != beforeRecv {
			t.Errorf("n=%d duplicate weight=%g gap=%d window=%g; want 0,0,%g", n, w, g, st.winRecv, beforeRecv)
		}
	}
}
