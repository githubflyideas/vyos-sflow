package settings

import (
	"bytes"
	"io"
	"net/netip"
	"strings"
	"testing"
)

type webPrompter interface {
	PromptWebAddress(bool, io.Reader, io.Writer, []netip.Addr) error
}
type forbiddenReader struct{}

func (forbiddenReader) Read([]byte) (int, error) { panic("headless/explicit startup read input") }

func TestOptionalWebPrompt(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		env         string
		tty         bool
		input, want string
		bad         bool
	}{
		{name: "headless", want: "127.0.0.1:8066"},
		{name: "blank", tty: true, input: "\n", want: "127.0.0.1:8066"},
		{name: "explicit all interfaces", tty: true, input: "0.0.0.0\n", want: "0.0.0.0:8066"},
		{name: "explicit all interfaces v6", tty: true, input: "::\n", want: "[::]:8066"},
		{name: "loopback", tty: true, input: "127.0.0.1\n", want: "127.0.0.1:8066"},
		{name: "loopback v6", tty: true, input: "::1\n", want: "[::1]:8066"},
		{name: "private local", tty: true, input: "192.168.1.2\n", want: "192.168.1.2:8066"},
		{name: "private v6", tty: true, input: "fd00::1\n", want: "[fd00::1]:8066"},
		{name: "not local", tty: true, input: "192.168.1.3\n", bad: true},
		{name: "public", tty: true, input: "8.8.8.8\n", bad: true},
		{name: "EOF", tty: true, bad: true},
		{name: "flag", tty: true, args: []string{"-addr", "127.0.0.1:9999"}, want: "127.0.0.1:9999"},
		{name: "environment", tty: true, env: "127.0.0.1:9998", want: "127.0.0.1:9998"},
		{name: "explicit wildcard flag", tty: true, args: []string{"-addr", "0.0.0.0:8066"}, want: "0.0.0.0:8066"},
		{name: "explicit wildcard environment", env: "0.0.0.0:8066", want: "0.0.0.0:8066"},
		{name: "garbage", tty: true, input: "router\n", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse(tc.args, func(k string) string {
				if k == "VYOS_SFLOW_ADDR" {
					return tc.env
				}
				return ""
			})
			if err != nil {
				t.Fatal(err)
			}
			p, ok := any(&c).(webPrompter)
			if !ok {
				t.Fatal("optional Web prompt is not implemented")
			}
			var in io.Reader = strings.NewReader(tc.input)
			skip := !tc.tty || len(tc.args) > 0 || tc.env != ""
			if skip {
				in = forbiddenReader{}
			}
			var out bytes.Buffer
			err = p.PromptWebAddress(tc.tty, in, &out, []netip.Addr{netip.MustParseAddr("192.168.1.2"), netip.MustParseAddr("fd00::1"), netip.MustParseAddr("8.8.8.8")})
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
			if !tc.bad && c.Addr != tc.want {
				t.Fatalf("addr=%s want=%s", c.Addr, tc.want)
			}
			if c.Listen != "0.0.0.0:6343" {
				t.Fatal("Web prompt altered UDP")
			}
			if skip && out.Len() != 0 {
				t.Fatal("headless/explicit startup prompted")
			}
		})
	}
}
