//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

func TestAProjectThemeThatNeverReadsDoesNotStallTheDaemon(t *testing.T) {
	d, _ := testDaemon(t)
	d.resolveTimeout = 50 * time.Millisecond

	themes := filepath.Join(paths.GlobalConfigDir(), "themes")
	if err := os.MkdirAll(themes, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(themes, "stuck.yaml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
	})

	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	root := project(t, "music:\n  theme: stuck\n")
	result := <-sendAsync(socket, protocol.Request{
		Event: &protocol.Event{Event: protocol.SessionStarted, ID: "stuck", Root: root},
	})
	if result.err != nil {
		t.Fatalf("session.started: %v", result.err)
	}
	if result.response.OK {
		t.Fatal("session.started with a theme that never reads = ok, want a timeout failure")
	}

	status := statusOf(t, socket)
	if len(status.Sessions) != 0 {
		t.Errorf("status reported %d sessions, want none", len(status.Sessions))
	}
	if status.Theme != "orchestra" {
		t.Errorf("theme = %q, want the default kept", status.Theme)
	}
}
