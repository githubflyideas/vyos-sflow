package main

import (
	"github.com/githubflyideas/vyos-sflow-new/internal/settings"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeadlessStartupNeverReadsInput(t *testing.T) {
	in, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	defer writer.Close()
	out, err := os.Create(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cfg, err := settings.Parse(nil, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- promptWebAddress(&cfg, in, out) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		in.Close()
		<-done
		t.Fatal("headless startup waited for input")
	}
	st, err := out.Stat()
	if err != nil || st.Size() != 0 {
		t.Fatalf("headless startup printed a prompt: %v", err)
	}
	if cfg.Addr != "0.0.0.0:8066" || cfg.Listen != "0.0.0.0:6343" {
		t.Fatalf("defaults changed: %+v", cfg)
	}
}
