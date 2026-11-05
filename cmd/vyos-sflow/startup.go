package main

import "log"

var version = "1.2.4"
var commit = "unknown"

func logStartup(scheme, addr, listen, data string) {
	log.Printf("vyos-sflow %s: %s://%s; sFlow UDP %s; data %s", version, scheme, addr, listen, data)
	log.Print("Security notice: bind HTTP and sFlow to a private/internal interface and restrict access with firewall rules. TLS is optional; configure -tls-cert and -tls-key to encrypt web access. No routes or firewall rules are changed automatically.")
	if scheme == "http" {
		log.Print("Security notice: HTTP traffic is unencrypted, including credentials; avoid untrusted networks.")
	}
}
