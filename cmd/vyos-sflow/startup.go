package main

import (
	"log"
	"net"
	"net/netip"
)

var version = "1.2.10"
var commit = "unknown"

func logStartup(scheme, addr, listen, data string) {
	log.Printf("vyos-sflow %s: %s://%s; sFlow UDP %s; data %s", version, scheme, addr, listen, data)
	log.Print("Security notice: bind HTTP and sFlow to a private/internal interface and restrict access with firewall rules. TLS is optional; configure -tls-cert and -tls-key to encrypt web access. No routes or firewall rules are changed automatically.")
	if scheme == "http" {
		log.Print("Security notice: HTTP traffic is unencrypted, including credentials; avoid untrusted networks.")
	}
	if loopbackOnly(addr) {
		log.Print("Web UI is reachable from this machine only. For remote access use -addr PRIVATE_IP:8066 (or VYOS_SFLOW_ADDR), or an SSH tunnel: ssh -N -L 8066:127.0.0.1:8066 SSH_USER@ROUTER_IP")
	}
}

func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}
