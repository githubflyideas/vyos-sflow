package pipeline

import (
	"fmt"
	"time"

	"github.com/githubflyideas/vyos-sflow-new/internal/store"
)

func (p *Pipeline) ingestQueued(q queuedRecords) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if q.epoch != p.epoch.Load() {
		return
	}
	p.noteDelay(q.records, time.Now())
	for i := range q.records {
		p.process(&q.records[i])
	}
}

// Purge clears all flow history, including queued records and detached flush
// batches. Only a zero time is accepted by the manual cleanup contract. The
// store's internal cutoff operation remains available for automatic retention.
// Identity/policy metadata, authentication, configuration and legacy business
// data are not flow history and are never cleared here.
func (p *Pipeline) Purge(before time.Time) (store.PurgeSize, error) {
	if !before.IsZero() {
		return store.PurgeSize{}, fmt.Errorf("manual cleanup only supports clearing all flow history")
	}
	p.queueMu.Lock()
	defer p.queueMu.Unlock()
	p.ioMu.Lock()
	defer p.ioMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epoch.Add(1)
	for {
		select {
		case <-p.in:
			continue
		default:
			goto drained
		}
	}
drained:
	p.forgetLocked(time.Time{})
	clear(p.dedup)
	p.newest = time.Time{}
	result, err := p.st.PurgeBefore(time.Time{})
	if err != nil {
		p.fail(err)
	}
	return result, err
}
