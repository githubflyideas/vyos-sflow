//go:build linux

// Command vyos-sflow is the Linux, sFlow-only traffic66 derivative.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/githubflyideas/vyos-sflow/internal/api"
	"github.com/githubflyideas/vyos-sflow/internal/auth"
	"github.com/githubflyideas/vyos-sflow/internal/bootstrap"
	"github.com/githubflyideas/vyos-sflow/internal/collector"
	"github.com/githubflyideas/vyos-sflow/internal/dnsres"
	"github.com/githubflyideas/vyos-sflow/internal/pipeline"
	"github.com/githubflyideas/vyos-sflow/internal/settings"
	"github.com/githubflyideas/vyos-sflow/internal/store"
	"github.com/githubflyideas/vyos-sflow/internal/tui"
	"github.com/githubflyideas/vyos-sflow/internal/web"
)

const usage = `vyos-sflow — Linux sFlow analytics with embedded DuckDB
Usage:
  vyos-sflow [serve] [flags]   receive sFlow and serve local analytics
  vyos-sflow tui [flags]       connect the terminal UI
  vyos-sflow passwd [flags]    set a persisted account password
  vyos-sflow version

Only sFlow v5 is accepted. NetFlow/IPFIX, packet capture and PCAP import are not included.
Use "vyos-sflow serve -h" for options. See packaging/ and docs/VYOS.md for installation.
`

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		log.Printf("vyos-sflow: %v", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		command = "version"
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		if command == "serve" && len(os.Args) <= 2 {
			command = "help"
		}
	}
	switch command {
	case "version":
		fmt.Printf("vyos-sflow %s (%s)\n", version, commit)
		return nil
	case "help":
		fmt.Print(usage)
		return nil
	case "serve":
		return serve(args)
	case "tui":
		return runTUI(args)
	case "passwd":
		return passwd(args)
	default:
		return fmt.Errorf("unknown command %q; use 'vyos-sflow help'", command)
	}
}

func serve(args []string) (result error) {
	cfg, err := settings.Parse(args, os.Getenv)
	if err != nil {
		return err
	}
	if err := promptWebAddress(&cfg, os.Stdin, os.Stdout); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveConfigured(ctx, cfg, os.Getenv("VYOS_SFLOW_PASSWORD"), nil)
}

// serveConfigured owns every resource it opens, and returns only after the
// producers have stopped, the pipeline has drained, and storage has closed.
func serveConfigured(ctx context.Context, cfg settings.Config, initialPassword string, ready chan<- tui.Options) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	checker, err := bootstrap.Ensure(cfg.Data, cfg.User, initialPassword)
	if err != nil {
		return err
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		lim := int64(float64(store.TotalMemory()) * cfg.MemoryFraction)
		if lim < 256<<20 {
			lim = 256 << 20
		}
		debug.SetMemoryLimit(lim)
	}
	st, err := store.Open(store.Options{Dir: cfg.Data, MemoryFraction: cfg.MemoryFraction, Threads: cfg.DBThreads, RawDays: 2, RollupDays: 2, AcceptRetentionChange: cfg.Accept48hRetention})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, st.Close()) }()
	pipe := pipeline.New(pipeline.Config{L2Overhead: 18}, st)
	col := collector.New(pipe)
	log.Print("sFlow header identities are recorded without sender filtering. Restrict UDP access with a firewall when needed.")
	col.Block, err = collector.NewBlocklist(cfg.Data)
	if err != nil {
		return err
	}
	listener, err := col.Listen(cfg.Listen, "sflow")
	if err != nil {
		return err
	}
	// Producers stop first. The pipeline's independent context remains live
	// until every producer has exited, and is drained before closing DuckDB.
	producers, stopProducers := context.WithCancel(context.Background())
	pipeCtx, stopPipe := context.WithCancel(context.Background())
	pipeDone := make(chan struct{})
	go func() { defer close(pipeDone); pipe.Run(pipeCtx) }()
	var wg sync.WaitGroup
	launch := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	launch(func() { col.Serve(producers, listener) })
	launch(func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-producers.Done():
				return
			case now := <-ticker.C:
				col.Tick(now)
			}
		}
	})
	defer func() {
		stopProducers()
		wg.Wait()
		stopPipe()
		<-pipeDone
		if err := col.Block.Flush(); err != nil {
			result = errors.Join(result, err)
		}
		if n := pipe.WriteErrs.Load(); n > 0 {
			result = errors.Join(result, fmt.Errorf("%d storage errors during this run: %s", n, pipe.LastError()))
		}
	}()
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	tokenPath := filepath.Join(cfg.Data, ".tui-token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return err
	}
	defer os.Remove(tokenPath)
	_, port, _ := net.SplitHostPort(listener.LocalAddr())
	srv := &api.Server{Store: st, Pipe: pipe, Col: col, DNS: dnsres.New(dnsres.Options{PerSecond: 2, TTL: 120 * time.Second}),
		Static: web.FS(), Version: version, BuildKind: "community", Check: checker.Check, Exists: checker.Exists, Accounts: checker, LocalTok: token, DataDir: cfg.Data, Started: time.Now(),
		Listen: []api.ListenInfo{{Name: "sflow", Port: port}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if st.DB.PingContext(ctx) != nil || pipe.WriteErrs.Load() != 0 {
			http.Error(w, "storage unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"status\":\"ok\",\"version\":%q}\n", version)
	})
	mux.Handle("/", srv.Handler())
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	scheme := "http"
	if cfg.TLSCert != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return err
		}
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		scheme = "https"
	}
	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	httpDone := make(chan error, 1)
	go func() { httpDone <- hs.Serve(ln) }()
	logStartup(scheme, ln.Addr().String(), listener.LocalAddr(), cfg.Data)
	if ready != nil {
		select {
		case ready <- tui.Options{URL: scheme + "://" + ln.Addr().String(), Token: token}:
		case <-ctx.Done():
		}
	}
	select {
	case <-ctx.Done():
	case err := <-httpDone:
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	}
	stopProducers()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := hs.Shutdown(shutdown); err != nil {
		hs.Close()
		result = errors.Join(result, err)
	}
	return result
}

func dataDefault() string {
	if d := os.Getenv("VYOS_SFLOW_DATA"); d != "" {
		return d
	}
	return settings.DefaultData
}
func runTUI(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	return runTUICommand(ctx, args, commandHooks{
		sessionHooks: sessionHooks{backend: temporaryBackend, view: tui.Run},
		terminal:     tui.IsTerminal, getenv: os.Getenv, available: localServiceAvailable, secret: tui.ReadSecret,
	})
}

func temporaryBackend(ctx context.Context, dir, listen string, ready chan<- tui.Options) error {
	// Ignore persistent-service environment settings. Only the newly created
	// owned directory receives this private, initialized session account.
	cfg, err := settings.Parse([]string{"-data", dir, "-addr", "127.0.0.1:0", "-listen", listen}, func(string) string { return "" })
	if err != nil {
		return err
	}
	return serveConfigured(ctx, cfg, auth.Generate(), ready)
}
func passwd(args []string) error {
	fs := flag.NewFlagSet("passwd", flag.ContinueOnError)
	data := fs.String("data", dataDefault(), "service data directory")
	user := fs.String("user", "admin", "account name")
	generate := fs.Bool("generate", false, "generate and print a new password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pw := os.Getenv("VYOS_SFLOW_PASSWORD")
	if *generate {
		pw = auth.Generate()
	} else if pw == "" {
		a, err := tui.ReadSecret("new password: ")
		if err != nil {
			return err
		}
		b, err := tui.ReadSecret("again: ")
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("passwords differ")
		}
		pw = a
	}
	if len(pw) < 12 {
		return fmt.Errorf("use at least 12 characters")
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	if err := auth.Set(*data, *user, pw); err != nil {
		return err
	}
	if *generate {
		fmt.Println(pw)
	}
	fmt.Println("password saved; restart the service to invalidate any existing sessions")
	return nil
}
