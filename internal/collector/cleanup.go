package collector

// ClearHistory runs the deletion of stored history with ingest held, so no
// record is submitted while it runs. Lock order is collector -> sink: the
// callback must not call back into Collector.
func (c *Collector) ClearHistory(clearHistory func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clearHistory()
}
