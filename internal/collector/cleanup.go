package collector

// ClearHistory serializes history deletion with sample estimation and sink
// submission. The callback must clear downstream queued, buffered and persisted
// history without calling back into Collector: lock order is collector -> sink.
// On failure the reservoir remains available; a rejected clear must not silently
// change the estimator. Source identity, calibration and diagnostics survive.
func (c *Collector) ClearHistory(clearHistory func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := clearHistory(); err != nil {
		return err
	}
	for _, st := range c.sf {
		st.recent = nil
		st.next = 0
	}
	return nil
}
