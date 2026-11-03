// Package enrich classifies endpoints and names port-based services (not DPI).
package enrich

import "net/netip"

var specialNets = mustPrefixes(
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10",
	"127.0.0.0/8", "169.254.0.0/16", "fc00::/7", "fe80::/10", "::1/128",
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// Internal classifies private, link-local, loopback and shared address space.
func Internal(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range specialNets {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
