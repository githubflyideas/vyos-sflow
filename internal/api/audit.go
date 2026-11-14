package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// audit writes one line of the audit trail to the service log: what was
// done, by whom, from which address. Every line starts with "audit:", so
// `journalctl -u vyos-sflow | grep audit:` lists them all. Passwords never
// appear in it; user names and other values a client chose are quoted, so
// none of them can forge a line of their own.
func audit(r *http.Request, event string, fields ...any) {
	var b strings.Builder
	b.WriteString("audit: ")
	b.WriteString(event)
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(&b, " %s=%q", fields[i], fmt.Sprint(fields[i+1]))
	}
	fmt.Fprintf(&b, " by=%q ip=%s", actorOf(r), clientIP(r))
	log.Print(b.String())
}

// actorOf names who made a request: the signed-in user, "local" for this
// machine's own token (the TUI and the subcommands), "" before sign-in.
func actorOf(r *http.Request) string {
	if u := userOf(r); u != "" {
		return u
	}
	if local(r) {
		return "local"
	}
	return ""
}
