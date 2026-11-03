package main

import (
	"net"
	"net/netip"
	"os"

	"github.com/githubflyideas/vyos-sflow-new/internal/settings"
)

func promptWebAddress(cfg *settings.Config, in, out *os.File) error {
	interactive := isTerminal(in) && isTerminal(out)
	var local []netip.Addr
	if interactive {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return err
		}
		for _, a := range addrs {
			if p, err := netip.ParsePrefix(a.String()); err == nil {
				local = append(local, p.Addr())
			}
		}
	}
	return cfg.PromptWebAddress(interactive, in, out, local)
}
