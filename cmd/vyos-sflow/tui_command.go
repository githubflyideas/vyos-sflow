package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/settings"
	"github.com/githubflyideas/vyos-sflow/internal/tui"
)

type commandHooks struct {
	sessionHooks
	terminal  func() bool
	getenv    func(string) string
	available func(context.Context) (bool, error)
	secret    func(string) (string, error)
}

func runTUICommand(ctx context.Context, args []string, h commandHooks) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	dataDefault := h.getenv("VYOS_SFLOW_DATA")
	if dataDefault == "" {
		dataDefault = settings.DefaultData
	}
	endpoint := fs.String("url", "http://127.0.0.1:8066", "existing service URL (attach only)")
	data := fs.String("data", dataDefault, "existing service data directory (attach only)")
	user := fs.String("user", "", "existing account authentication (attach only)")
	lang := fs.String("lang", "", "language code")
	listen := fs.String("listen", "0.0.0.0:6343", "temporary session sFlow UDP address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if !h.terminal() {
		return errors.New("vyos-sflow tui needs an interactive terminal")
	}
	attach := h.getenv("VYOS_SFLOW_DATA") != ""
	listenExplicit := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "url", "user", "data":
			attach = true
		case "listen":
			listenExplicit = true
		}
	})
	if !attach {
		// A service on another port says where it listens in its data
		// directory; it counts when it answers there.
		if b, err := os.ReadFile(filepath.Join(*data, ".tui-url")); err == nil {
			if u, err := url.Parse(strings.TrimSpace(string(b))); err == nil && u.Host != "" {
				if ok, _ := serviceAvailableAt(ctx, u.Host); ok {
					attach = true
				}
			}
		}
	}
	if !attach {
		var err error
		attach, err = h.available(ctx)
		if err != nil {
			return fmt.Errorf("cannot determine whether the local service is available: %w", err)
		}
	}
	if !attach {
		return runOwnedTUI(ctx, *listen, *lang, h.sessionHooks)
	}
	if listenExplicit {
		return errors.New("-listen applies only to a new temporary session; omit it when attaching")
	}
	urlGiven := false
	fs.Visit(func(f *flag.Flag) { urlGiven = urlGiven || f.Name == "url" })
	if !urlGiven {
		// the running service says where it listens
		if b, err := os.ReadFile(filepath.Join(*data, ".tui-url")); err == nil && strings.TrimSpace(string(b)) != "" {
			*endpoint = strings.TrimSpace(string(b))
		}
	}
	opt := tui.Options{URL: *endpoint, User: *user, Lang: *lang}
	if *user != "" {
		opt.Password = h.getenv("VYOS_SFLOW_PASSWORD")
		if opt.Password == "" {
			var err error
			opt.Password, err = h.secret("password: ")
			if err != nil {
				return err
			}
		}
	} else {
		b, err := os.ReadFile(filepath.Join(*data, ".tui-token"))
		if err != nil {
			return fmt.Errorf("cannot read existing service token; use -data or -user to attach: %w", err)
		}
		opt.Token = strings.TrimSpace(string(b))
		if opt.Token == "" {
			return errors.New("existing service token is empty; use -user to attach")
		}
	}
	return h.view(ctx, opt)
}

// Only a definite connection refusal means absent. Timeouts, permission errors
// and a listening but unauthenticated/unrelated server must never trigger a
// second collector. Discovery neither writes data nor sends credentials.
func localServiceAvailable(ctx context.Context) (bool, error) {
	return serviceAvailableAt(ctx, "127.0.0.1:8066")
}
func serviceAvailableAt(ctx context.Context, addr string) (bool, error) {
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", addr)
	if err == nil {
		conn.Close()
		return true, nil
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return false, nil
	}
	return false, err
}
