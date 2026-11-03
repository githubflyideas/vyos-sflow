// vmguest is a test-only, statically compiled guest probe. It has no product
// imports, third-party modules, interpreter, or runtime installation dependency.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type header struct {
	Version  uint32   `json:"version"`
	Agent    string   `json:"agent"`
	SubAgent uint32   `json:"sub_agent"`
	Sequence uint32   `json:"sequence"`
	Uptime   uint32   `json:"uptime_ms"`
	Samples  uint32   `json:"samples"`
	Hex      string   `json:"header_hex"`
	Tags     []string `json:"sample_headers_hex"`
}

func decodeDatagram(b []byte) (header, error) {
	h := header{}
	invalid := errors.New("invalid bounded sFlow datagram")
	if len(b) < 8 || binary.BigEndian.Uint32(b) != 5 {
		return h, invalid
	}
	size := 0
	switch binary.BigEndian.Uint32(b[4:]) {
	case 1:
		size = 4
	case 2:
		size = 16
	default:
		return h, invalid
	}
	if len(b) < 24+size {
		return h, invalid
	}
	offset := 8 + size
	h = header{Version: 5, Agent: net.IP(b[8:offset]).String(), SubAgent: binary.BigEndian.Uint32(b[offset:]), Sequence: binary.BigEndian.Uint32(b[offset+4:]), Uptime: binary.BigEndian.Uint32(b[offset+8:]), Samples: binary.BigEndian.Uint32(b[offset+12:]), Hex: hex.EncodeToString(b[:offset+16]), Tags: []string{}}
	if h.Samples > 64 {
		return header{}, invalid
	}
	offset += 16
	for i := uint32(0); i < h.Samples; i++ {
		if offset+8 > len(b) {
			return header{}, invalid
		}
		length := uint64(binary.BigEndian.Uint32(b[offset+4:]))
		h.Tags = append(h.Tags, hex.EncodeToString(b[offset:offset+8]))
		offset += 8
		if length > uint64(len(b)-offset) {
			return header{}, invalid
		}
		offset += int(length)
	}
	return h, nil
}
func (h header) flow() bool {
	for _, t := range h.Tags {
		if strings.HasPrefix(t, "00000001") || strings.HasPrefix(t, "00000003") {
			return true
		}
	}
	return false
}

// Own the socket and close it on every success, timeout, malformed packet and
// cancellation path. Never enable SO_REUSEPORT or run alongside the collector.
func capture(ctx context.Context, conn net.PacketConn, timeout time.Duration) ([]header, error) {
	defer conn.Close()
	rows := []header{}
	flows, counters := 0, 0
	end := time.Now().Add(timeout)
	buf := make([]byte, 65535)
	for flows < 3 {
		if ctx.Err() != nil {
			return rows, ctx.Err()
		}
		conn.SetReadDeadline(end)
		n, _, e := conn.ReadFrom(buf)
		if e != nil {
			return rows, errors.New("native probe timeout or read failure")
		}
		h, e := decodeDatagram(buf[:n])
		if e != nil {
			return rows, e
		}
		if h.flow() {
			rows = append(rows, h)
			flows++
		} else if counters < 3 {
			rows = append(rows, h)
			counters++
		}
	}
	return rows, nil
}
func probe(url string) ([]header, error) {
	conn, e := net.ListenPacket("udp4", "127.0.0.1:6343")
	if e != nil {
		return nil, errors.New("probe bind failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	// Only HTTP packets on eth0. There is no sFlow packet generator in this helper.
	go func() {
		defer close(done)
		client := &http.Client{Timeout: 3 * time.Second}
		for n := 0; n < 30; n++ {
			req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
			if e != nil {
				return
			}
			r, e := client.Do(req)
			if e == nil {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
	rows, e := capture(ctx, conn, 45*time.Second)
	cancel()
	<-done
	return rows, e
}

type checker struct {
	base, data, temporary, password, checkpoint string
	client                                      *http.Client
	lastStatus                                  int
	waitDuration, interval                      time.Duration
}

func newChecker(base, data, temporary string) *checker {
	return &checker{base: base, data: data, temporary: temporary, client: &http.Client{Timeout: 5 * time.Second}, checkpoint: "guest_check", waitDuration: 120 * time.Second, interval: 2 * time.Second}
}
func (c *checker) request(method, path string, payload any, authenticated bool) (map[string]any, int, error) {
	var body io.Reader
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return nil, 0, e
		}
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, c.base+path, body)
	if e != nil {
		return nil, 0, errors.New("invalid request")
	}
	if authenticated {
		req.SetBasicAuth("admin", c.password)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r, e := c.client.Do(req)
	if e != nil {
		return nil, 0, errors.New("HTTP transport failure")
	}
	defer r.Body.Close()
	c.lastStatus = r.StatusCode
	if r.StatusCode != 200 {
		return nil, r.StatusCode, errors.New("unexpected HTTP status")
	}
	// No response body, request or credential enters an error string or traceback.
	d := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	d.UseNumber()
	var value map[string]any
	if e = d.Decode(&value); e != nil {
		return nil, r.StatusCode, errors.New("invalid JSON response")
	}
	return value, r.StatusCode, nil
}
func (c *checker) get(path string) (map[string]any, error) {
	v, _, e := c.request("GET", path, nil, true)
	return v, e
}
func unsigned(value any) (uint64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	v, e := strconv.ParseUint(string(n), 10, 64)
	return v, e == nil
}
func (c *checker) total() (uint64, error) {
	v, e := c.get("/api/overview?range=1h")
	if e != nil {
		return 0, e
	}
	totals, ok := v["totals"].(map[string]any)
	if !ok {
		return 0, errors.New("missing totals")
	}
	wire, ok := unsigned(totals["wire"])
	if !ok {
		return 0, errors.New("invalid wire total")
	}
	return wire, nil
}
func (c *checker) wait(fn func() bool) error {
	end := time.Now().Add(c.waitDuration)
	for time.Now().Before(end) {
		if fn() {
			return nil
		}
		time.Sleep(c.interval)
	}
	return errors.New("check deadline exceeded")
}
func (c *checker) run(mode string) (map[string]any, error) {
	file := filepath.Join(c.temporary, "vyos-vm-login")
	credential, e := os.ReadFile(file)
	fresh := errors.Is(e, os.ErrNotExist)
	if e != nil && !fresh {
		return nil, errors.New("credential unavailable")
	}
	c.password = string(credential)
	if fresh {
		c.password = "tryvyossflow66"
	}
	c.checkpoint = "health_wait"
	if e = c.wait(func() bool {
		v, _, e := c.request("GET", "/healthz", nil, false)
		return e == nil && v["status"] == "ok"
	}); e != nil {
		return nil, e
	}
	switch mode {
	case "baseline":
		c.checkpoint = "baseline"
		if fresh {
			secret := make([]byte, 24)
			if _, e = rand.Read(secret); e != nil {
				return nil, e
			}
			next := base64.RawURLEncoding.EncodeToString(secret)
			if _, _, e = c.request("POST", "/api/password", map[string]string{"old": c.password, "new": next}, true); e != nil {
				return nil, e
			}
			if e = os.WriteFile(file, []byte(next), 0600); e != nil {
				return nil, errors.New("credential persistence failed")
			}
			c.password = next
		}
		_, code, _ := c.request("GET", "/api/overview?range=1h", nil, false)
		if code != 401 {
			return nil, errors.New("unauthenticated analytics not rejected")
		}
		n, e := c.total()
		if e != nil {
			return nil, e
		}
		if n != 0 {
			return nil, errors.New("baseline contains traffic")
		}
		return map[string]any{"health": "ok", "unauthenticated_status": 401, "wire": n}, nil
	case "native":
		c.checkpoint = "native_wait"
		var wire uint64
		if e = c.wait(func() bool { var err error; wire, err = c.total(); return err == nil && wire > 0 }); e != nil {
			return nil, e
		}
		c.checkpoint = "native_sources"
		sources, e := c.get("/api/sources")
		if e != nil {
			return nil, e
		}
		rows, ok := sources["sources"].([]any)
		accepted := false
		if ok {
			for _, row := range rows {
				r, ok := row.(map[string]any)
				if !ok {
					continue
				}
				packets, p := unsigned(r["packets"])
				records, f := unsigned(r["records"])
				if r["proto"] == "sFlow v5" && p && f && packets > 0 && records > 0 {
					accepted = true
				}
			}
		}
		if !accepted {
			return nil, errors.New("no native flow source")
		}
		c.checkpoint = "native_storage"
		files, e := filepath.Glob(filepath.Join(c.data, "*.duckdb"))
		if e != nil || len(files) == 0 {
			return nil, errors.New("DuckDB missing")
		}
		return map[string]any{"wire": wire, "sources": safe(sources, 0), "duckdb_present": true}, nil
	case "stream":
		c.checkpoint = "native_sources"
		sources, e := c.get("/api/sources")
		if e != nil {
			return nil, e
		}
		return map[string]any{"sources": safe(sources, 0)}, nil
	case "snapshot":
		c.checkpoint = "snapshot"
		n, e := c.total()
		if e != nil {
			return nil, e
		}
		if n == 0 {
			return nil, errors.New("empty persisted history")
		}
		if e = os.WriteFile(filepath.Join(c.temporary, "vyos-vm-history"), []byte(strconv.FormatUint(n, 10)), 0600); e != nil {
			return nil, errors.New("snapshot write failed")
		}
		return map[string]any{"persisted_wire": n}, nil
	case "history":
		c.checkpoint = "history"
		raw, e := os.ReadFile(filepath.Join(c.temporary, "vyos-vm-history"))
		if e != nil {
			return nil, errors.New("snapshot missing")
		}
		expected, e := strconv.ParseUint(string(raw), 10, 64)
		if e != nil {
			return nil, errors.New("invalid snapshot")
		}
		actual, e := c.total()
		if e != nil {
			return nil, e
		}
		if expected != actual {
			return nil, errors.New("history lost or duplicated")
		}
		return map[string]any{"persisted_wire": actual, "expected_wire": expected}, nil
	default:
		return nil, errors.New("unknown check mode")
	}
}
func safe(value map[string]any, depth int) map[string]any {
	out := map[string]any{}
	if depth > 4 {
		return out
	}
	numbers := " sub_agent rejected_streams packets records decode_errors undecoded dropped panics counter_samples write_errors hot_rows disk_bytes sources_http_status status_http_status http_status hsflowd_processes exit_code "
	for k, v := range value {
		if strings.Contains(numbers, " "+k+" ") {
			if n, ok := unsigned(v); ok {
				out[k] = n
			}
			continue
		}
		switch k {
		case "agent", "exporter":
			if s, ok := v.(string); ok && net.ParseIP(s) != nil {
				out[k] = net.ParseIP(s).String()
			}
		case "configured", "enforced":
			if b, ok := v.(bool); ok {
				out[k] = b
			}
		case "proto":
			if v == "sFlow v5" || v == "sflow" {
				out[k] = v
			}
		case "sources", "listeners", "stream", "status":
			switch child := v.(type) {
			case map[string]any:
				out[k] = safe(child, depth+1)
			case []any:
				rows := []any{}
				for i, row := range child {
					if i >= 8 {
						break
					}
					if m, ok := row.(map[string]any); ok {
						rows = append(rows, safe(m, depth+1))
					}
				}
				out[k] = rows
			}
		}
	}
	return out
}
func reportFailure(out io.Writer, checkpoint string, fields map[string]any) {
	d := safe(fields, 0)
	allowed := " health_wait baseline native_wait native_sources native_storage snapshot history native_probe_failed guest_check "
	if !strings.Contains(allowed, " "+checkpoint+" ") {
		checkpoint = "guest_check"
	}
	d["code"] = checkpoint
	d["error_type"] = "Error"
	fmt.Fprint(out, "VM_DIAGNOSTIC ")
	json.NewEncoder(out).Encode(d)
}
func (c *checker) diagnostics() map[string]any {
	fields := map[string]any{}
	if c.lastStatus != 0 {
		fields["http_status"] = json.Number(strconv.Itoa(c.lastStatus))
	}
	for _, item := range []struct{ name, path string }{{"sources", "/api/sources"}, {"status", "/api/status"}} {
		v, status, e := c.request("GET", item.path, nil, true)
		if e == nil {
			fields[item.name] = v
		} else if status > 0 {
			fields[item.name+"_http_status"] = json.Number(strconv.Itoa(status))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "pgrep", "-x", "hsflowd").Output()
	if e == nil {
		fields["hsflowd_processes"] = json.Number(strconv.Itoa(len(strings.Fields(string(raw)))))
	}
	return fields
}
func execute(args []string, out io.Writer) (code int) {
	// Recover without panic text/stack: even unexpected guest failures are safe.
	checkpoint := "guest_check"
	defer func() {
		if recover() != nil {
			reportFailure(out, checkpoint, nil)
			code = 1
		}
	}()
	if len(args) < 1 || (args[0] != "probe" && args[0] != "baseline" && args[0] != "native" && args[0] != "snapshot" && args[0] != "history" && args[0] != "stream") {
		reportFailure(out, checkpoint, nil)
		return 1
	}
	if args[0] == "probe" {
		checkpoint = "native_probe_failed"
		if len(args) != 2 {
			reportFailure(out, checkpoint, nil)
			return 1
		}
		rows, e := probe(args[1])
		if e != nil {
			// Rows contain only validated fixed/sample headers, never payloads.
			fmt.Fprint(out, "VM_DIAGNOSTIC ")
			json.NewEncoder(out).Encode(map[string]any{"code": checkpoint, "error_type": "Error", "headers": rows})
			return 1
		}
		json.NewEncoder(out).Encode(map[string]any{"headers": rows})
		return 0
	}
	c := newChecker("http://127.0.0.1:8066", "/config/vyos-sflow/data", "/tmp")
	value, e := c.run(args[0])
	checkpoint = c.checkpoint
	if e != nil {
		reportFailure(out, checkpoint, c.diagnostics())
		return 1
	}
	json.NewEncoder(out).Encode(value)
	return 0
}
func main() { os.Exit(execute(os.Args[1:], os.Stdout)) }
