package collector

import (
	"fmt"
	"net/netip"
)

// SetAllowedStream enables explicit header identity admission. Empty agent is
// fail-closed, not first-seen binding. Call before Listen/Serve. The header is
// self-reported and this policy is NOT source authentication.
// A configured collector cannot be rebound; restart with the new configuration.
func (c *Collector) SetAllowedStream(agent string, sub uint32) error {
	var a netip.Addr
	if agent != "" {
		var err error
		a, err = netip.ParseAddr(agent)
		if err != nil || a.Zone() != "" {
			return fmt.Errorf("agent must be a numeric IP without a zone")
		}
		a = a.Unmap()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streamEnforced {
		if c.allowedAgent == a && c.allowedSubAgent == sub {
			return nil
		}
		return fmt.Errorf("stream policy is immutable; restart with the new configuration")
	}
	if len(c.sfAgents) != 0 || len(c.sf) != 0 {
		return fmt.Errorf("configure stream before receiving datagrams")
	}
	c.streamEnforced, c.allowedAgent, c.allowedSubAgent = true, a, sub
	return nil
}

type StreamStatus struct {
	Enforced   bool   `json:"enforced"`
	Configured bool   `json:"configured"`
	Agent      string `json:"agent"`
	SubAgent   uint32 `json:"sub_agent"`
	Rejected   uint64 `json:"rejected_streams"`
	Message    string `json:"message,omitempty"`
}

func (c *Collector) StreamStatus() StreamStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := StreamStatus{Enforced: c.streamEnforced, Configured: c.allowedAgent.IsValid(), SubAgent: c.allowedSubAgent, Rejected: c.RejectedStreams.Load()}
	if s.Configured {
		s.Agent = c.allowedAgent.String()
	}
	if s.Enforced && !s.Configured {
		s.Message = "explicit internal stream policy has no identity; all samples are rejected"
	}
	return s
}
