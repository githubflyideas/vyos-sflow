package settings

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
)

// PromptWebAddress changes only the Web listener. Explicit flags/environment
// and non-interactive callers never read stdin or emit a prompt.
func (c *Config) PromptWebAddress(interactive bool, in io.Reader, out io.Writer, local []netip.Addr) error {
	if !interactive || c.addrExplicit {
		return nil
	}
	if _, err := fmt.Fprint(out, "Web private local IP (Enter for all interfaces 0.0.0.0): "); err != nil {
		return err
	}
	line, err := bufio.NewReader(io.LimitReader(in, 1024)).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read Web address: %w", err)
	}
	host := strings.TrimSpace(line)
	if host == "" {
		host = "0.0.0.0"
	} else {
		addr, err := netip.ParseAddr(host)
		if err != nil || addr.Zone() != "" || !addr.IsPrivate() {
			return fmt.Errorf("Web address must be a private local IP, or Enter for all interfaces")
		}
		found := false
		for _, a := range local {
			if a.Unmap() == addr.Unmap() {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Web address %s is not assigned to a local interface", host)
		}
		host = addr.Unmap().String()
	}
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return err
	}
	c.Addr = net.JoinHostPort(host, port)
	return nil
}
