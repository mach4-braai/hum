//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestBinaryTerminatesGracefullyWithAnActiveSession(t *testing.T) {
	p := startProcess(t)

	started := protocol.Event{Event: protocol.SessionStarted, ID: "live", Title: "long work"}
	if responses := send(t, p.socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v", responses[0])
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	if code := waitExit(t, p.cmd, 30*time.Second); code != exitOK {
		t.Errorf("humd exited %d after SIGTERM, want %d", code, exitOK)
	}
	if _, err := os.Stat(p.socket); err == nil {
		t.Errorf("socket %s survived a clean shutdown", p.socket)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.socket), "humd.pid")); err == nil {
		t.Error("pidfile survived a clean shutdown")
	}
}

func TestBinarySecondSignalExitsNonZero(t *testing.T) {
	p := startProcess(t)

	started := protocol.Event{Event: protocol.SessionStarted, ID: "live"}
	if responses := send(t, p.socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v", responses[0])
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("first signal: %v", err)
	}
	p.waitForLog(t, "waiting for voices to fade", 10*time.Second)

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("second signal: %v", err)
	}

	if code := waitExit(t, p.cmd, 10*time.Second); code != exitInterrupted {
		t.Errorf("humd exited %d after a second signal, want %d", code, exitInterrupted)
	}
}
