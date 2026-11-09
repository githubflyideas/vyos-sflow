package settings

import (
	"crypto/tls"
	"flag"
	"fmt"
	"math"
	"net"
	"net/netip"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultData keeps its historical spelling ("vyso") on purpose: existing
// direct-execution installs store accounts and history there, and renaming
// it would silently start new users with an empty directory.
const DefaultData = "./vyso_sflow_data"

// DefaultAddr keeps the Web UI on loopback unless an address is chosen
// explicitly (flag, environment or the interactive prompt). sFlow keeps
// DefaultListen on all interfaces so remote exporters keep working.
const (
	DefaultAddr   = "127.0.0.1:8066"
	DefaultListen = "0.0.0.0:6343"
)

// Config is the Linux service's explicitly configured runtime boundary.
type Config struct {
	addrExplicit                              bool
	Accept48hRetention                        bool
	Addr, Listen, Data, User, TLSCert, TLSKey string
	RetentionDays, DBThreads                  int
	MemoryFraction                            float64
	// AllowRemoteHTTP is a deprecated compatibility field and has no effect.
	AllowRemoteHTTP bool
}

// Parse validates environment values first; explicit flags then override them.
// Passwords are deliberately not flags or fields that a diagnostic can print.
func Parse(args []string, getenv func(string) string) (Config, error) {
	c := Config{Addr: DefaultAddr, Listen: DefaultListen, Data: DefaultData, User: "admin", RetentionDays: 2, DBThreads: 1, MemoryFraction: 0.05}
	fs := flag.NewFlagSet("vyos-sflow serve", flag.ContinueOnError)

	fs.BoolVar(&c.Accept48hRetention, "accept-48h-retention", false, "explicitly accept migration of existing history to rolling 48-hour retention")
	fs.StringVar(&c.Addr, "addr", c.Addr, "web/API address (loopback 127.0.0.1 by default; use a private interface IP or 0.0.0.0 for remote access)")
	fs.StringVar(&c.Listen, "listen", c.Listen, "one sFlow-only UDP address (all interfaces by default; private interface recommended)")
	fs.StringVar(&c.Data, "data", c.Data, "persistent data directory")
	fs.StringVar(&c.User, "user", c.User, "initial administrator")
	fs.StringVar(&c.TLSCert, "tls-cert", "", "TLS certificate PEM")
	fs.StringVar(&c.TLSKey, "tls-key", "", "TLS private key PEM")
	fs.IntVar(&c.RetentionDays, "retention-days", c.RetentionDays, "history retention days (must be 2; rolling 48 hours)")
	fs.IntVar(&c.DBThreads, "db-threads", c.DBThreads, "DuckDB query threads")
	fs.Float64Var(&c.MemoryFraction, "memory", c.MemoryFraction, "database memory fraction; Go uses a separate soft budget")
	fs.BoolVar(&c.AllowRemoteHTTP, "allow-remote-http", false, "deprecated no-op; HTTP is allowed on any configured interface")
	for env, name := range map[string]string{"ACCEPT_48H_RETENTION": "accept-48h-retention", "ADDR": "addr", "LISTEN": "listen", "DATA": "data", "USER": "user", "TLS_CERT": "tls-cert", "TLS_KEY": "tls-key", "RETENTION_DAYS": "retention-days", "DB_THREADS": "db-threads", "MEMORY_FRACTION": "memory", "ALLOW_REMOTE_HTTP": "allow-remote-http"} {
		if value := getenv("VYOS_SFLOW_" + env); value != "" {
			if err := fs.Set(name, value); err != nil {
				return c, fmt.Errorf("invalid VYOS_SFLOW_%s: %w", env, err)
			}
		}
	}
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			c.addrExplicit = true
		}
	})

	if c.RetentionDays != 2 {
		return c, fmt.Errorf("retention-days must be 2 (rolling 48 hours)")
	}
	if c.DBThreads < 1 || c.DBThreads > 256 {
		return c, fmt.Errorf("db-threads must be between 1 and 256")
	}
	if math.IsNaN(c.MemoryFraction) || math.IsInf(c.MemoryFraction, 0) || c.MemoryFraction <= 0 || c.MemoryFraction > 0.5 {
		return c, fmt.Errorf("memory must be finite and in (0,0.5]")
	}
	if c.Data == "" {
		return c, fmt.Errorf("data directory must not be empty")
	}
	if !path.IsAbs(c.Data) {
		abs, err := filepath.Abs(c.Data)
		if err != nil {
			return c, fmt.Errorf("resolve data directory: %w", err)
		}
		c.Data = abs
	}
	if c.User == "" || len(c.User) > 64 || strings.ContainsAny(c.User, ": \t\r\n") {
		return c, fmt.Errorf("invalid initial user")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return c, fmt.Errorf("tls-cert and tls-key must be configured together")
	}
	if c.TLSCert != "" {
		if _, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey); err != nil {
			return c, fmt.Errorf("invalid TLS certificate/key pair: %w", err)
		}
	}
	for _, addr := range []string{c.Listen, c.Addr} {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return c, fmt.Errorf("invalid listen address %q: %w", addr, err)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return c, fmt.Errorf("invalid port in %q", addr)
		}
		if host != "" && host != "localhost" {
			if _, err := netip.ParseAddr(host); err != nil {
				return c, fmt.Errorf("address must use a numeric IP or localhost: %q", addr)
			}
		}
	}
	return c, nil
}
