package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Synthetic OFFLINE transport fixtures; never presented as native VM evidence.
func sample(sub, tag uint32, ipv6 bool) []byte {
	addr := net.ParseIP("10.0.2.15").To4()
	family := uint32(1)
	if ipv6 {
		addr = net.ParseIP("2001:db8::1").To16()
		family = 2
	}
	b := new(bytes.Buffer)
	for _, v := range []uint32{5, family} {
		_ = binary.Write(b, binary.BigEndian, v)
	}
	b.Write(addr)
	payload := []byte("PRIVATE PAYLOAD!")
	for _, v := range []uint32{sub, 9, 2000, 1, tag, uint32(len(payload))} {
		_ = binary.Write(b, binary.BigEndian, v)
	}
	b.Write(payload)
	return b.Bytes()
}
func TestHeader(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		b := sample(123456, 1, v6)
		h, e := decodeDatagram(b)
		if e != nil || h.SubAgent != 123456 || !h.flow() {
			t.Fatalf("decode: %+v %v", h, e)
		}
		j, _ := json.Marshal(h)
		if strings.Contains(string(j), "PRIVATE") {
			t.Fatal("payload leak")
		}
		for n := 0; n < len(b); n++ {
			if _, e := decodeDatagram(b[:n]); e == nil {
				t.Fatalf("accepted truncation %d", n)
			}
		}
	}
	for _, b := range [][]byte{nil, make([]byte, 28)} {
		if _, e := decodeDatagram(b); e == nil {
			t.Fatal("bad header accepted")
		}
	}
}
func TestProbeRealUDPClosesSocketAndBoundsCounterEvidence(t *testing.T) {
	conn, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := conn.LocalAddr().String()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender, e := net.Dial("udp4", addr)
		if e != nil {
			return
		}
		defer sender.Close()
		for n := 0; n < 8; n++ {
			sender.Write(sample(123457, 2, false))
		}
		for n := 0; n < 3; n++ {
			sender.Write(sample(123456, 1, false))
		}
	}()
	headers, e := capture(context.Background(), conn, 2*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	<-done
	flows, counters := 0, 0
	for _, h := range headers {
		if h.flow() {
			flows++
		} else {
			counters++
		}
	}
	if flows != 3 || counters != 3 {
		t.Fatalf("wrong evidence bounds %d %d", flows, counters)
	}
	next, e := net.ListenPacket("udp4", addr)
	if e != nil {
		t.Fatal("probe socket not closed", e)
	}
	next.Close()
}
func TestProbeTimeoutFailsClosed(t *testing.T) {
	c, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = capture(context.Background(), c, 20*time.Millisecond); e == nil {
		t.Fatal("empty probe accepted")
	}
}
func TestExecuteInvalidArgumentsNeverLeaks(t *testing.T) {
	out := new(bytes.Buffer)
	if execute([]string{"PRIVATE-request-auth"}, out) != 1 {
		t.Fatal("invalid mode accepted")
	}
	if strings.Contains(out.String(), "PRIVATE") || !strings.Contains(out.String(), "VM_DIAGNOSTIC") {
		t.Fatal(out.String())
	}
}

func TestCredentialSafeFailure(t *testing.T) {
	b := new(bytes.Buffer)
	reportFailure(b, "native_wait", map[string]any{"write_errors": json.Number("2"), "last_error": "PRIVATE", "password": "PRIVATE", "stream": map[string]any{"agent": "10.0.2.15", "sub_agent": json.Number("123456"), "configured": true}})
	if strings.Contains(b.String(), "PRIVATE") || !strings.Contains(b.String(), "write_errors") {
		t.Fatal(b.String())
	}
}
func TestGuestLifecycleAndActualSourceProtocol(t *testing.T) {
	root := t.TempDir()
	password := "tryvyossflow66"
	wire := "0"
	proto := "sFlow v5"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			io.WriteString(w, `{"status":"ok"}`)
			return
		}
		user, p, ok := r.BasicAuth()
		if !ok || user != "admin" || p != password {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/password":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["old"] != password || body["new"] == "" {
				w.WriteHeader(400)
				return
			}
			password = body["new"]
			io.WriteString(w, `{}`)
		case "/api/overview":
			io.WriteString(w, `{"totals":{"wire":`+wire+`}}`)
		case "/api/sources":
			json.NewEncoder(w).Encode(map[string]any{"sources": []any{map[string]any{"proto": proto, "packets": 2, "records": 3}}, "stream": map[string]any{"agent": "10.0.2.15", "sub_agent": 123456, "configured": true, "enforced": true, "rejected_streams": 2}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := newChecker(srv.URL, root, root)
	c.waitDuration = 100 * time.Millisecond
	c.interval = time.Millisecond
	baseline, e := c.run("baseline")
	if e != nil || baseline["wire"] != uint64(0) {
		t.Fatal(baseline, e)
	}
	credential, e := os.ReadFile(filepath.Join(root, "vyos-vm-login"))
	if e != nil || string(credential) != password || password == "tryvyossflow66" {
		t.Fatal("password not changed privately", e)
	}
	wire = "123"
	os.WriteFile(filepath.Join(root, "flows.duckdb"), nil, 0600)
	native, e := c.run("native")
	if e != nil || native["wire"] != uint64(123) {
		t.Fatal(native, e)
	}
	proto = "sflow"
	if _, e = c.run("native"); e == nil {
		t.Fatal("accepted listener protocol as source protocol")
	}
	proto = "sFlow v5"
	if _, e = c.run("snapshot"); e != nil {
		t.Fatal(e)
	}
	// Recreate checker to prove disk snapshot and credentials, not in-memory state.
	c = newChecker(srv.URL, root, root)
	if _, e = c.run("history"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.run("stream"); e != nil {
		t.Fatal("reopened stream check", e)
	}
	wire = "124"
	if _, e = c.run("history"); e == nil {
		t.Fatal("lost/duplicated history accepted")
	}
}
