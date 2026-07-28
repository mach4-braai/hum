package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

var buildHumd = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "humd-bin")
	if err != nil {
		return "", err
	}
	binary := filepath.Join(dir, "humd")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		return "", errors.New(string(out))
	}
	return binary, nil
})

type process struct {
	cmd    *exec.Cmd
	socket string
	lines  chan string
}

func startProcess(t *testing.T, args ...string) *process {
	t.Helper()

	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}

	home, err := os.MkdirTemp("", "hh")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	socket := filepath.Join(home, "humd.sock")

	cmd := exec.Command(binary, append([]string{"--no-audio", "--log-level", "debug", "--socket", socket}, args...)...)
	cmd.Env = append(os.Environ(), "HUM_HOME="+home)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start humd: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})

	lines := make(chan string, 4096)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	waitForSocket(t, socket)
	return &process{cmd: cmd, socket: socket, lines: lines}
}

func (p *process) waitForLog(t *testing.T, substring string, within time.Duration) {
	t.Helper()

	deadline := time.After(within)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				t.Fatalf("humd stderr closed before logging %q", substring)
			}
			if strings.Contains(line, substring) {
				return
			}
		case <-deadline:
			t.Fatalf("humd did not log %q within %v", substring, within)
		}
	}
}

func waitExit(t *testing.T, cmd *exec.Cmd, within time.Duration) int {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("wait: %v", err)
		}
		return exitErr.ExitCode()
	case <-time.After(within):
		cmd.Process.Kill()
		t.Fatalf("humd did not exit within %v", within)
	}
	return -1
}

func TestBinaryVersionExitsZero(t *testing.T) {
	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}

	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("humd --version: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Error("humd --version printed nothing")
	}
}

func TestBinaryUsageErrorExitsTwo(t *testing.T) {
	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}

	err = exec.Command(binary, "--nonsense").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("humd --nonsense = %v, want a non-zero exit", err)
	}
	if exitErr.ExitCode() != exitUsage {
		t.Errorf("humd --nonsense exited %d, want %d", exitErr.ExitCode(), exitUsage)
	}
}

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

func TestBinaryShutdownCommandStopsTheProcess(t *testing.T) {
	p := startProcess(t)

	if responses := send(t, p.socket, protocol.Request{Command: protocol.CmdShutdown}); !responses[0].OK {
		t.Fatalf("shutdown = %+v", responses[0])
	}

	if code := waitExit(t, p.cmd, 30*time.Second); code != exitOK {
		t.Errorf("humd exited %d after the shutdown command, want %d", code, exitOK)
	}
	if _, err := os.Stat(p.socket); err == nil {
		t.Errorf("socket %s survived the shutdown command", p.socket)
	}
}

func TestBinaryRefusesASecondInstance(t *testing.T) {
	p := startProcess(t)

	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}
	second := exec.Command(binary, "--no-audio", "--socket", p.socket)
	second.Env = append(os.Environ(), "HUM_HOME="+filepath.Dir(p.socket))
	out, err := second.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("second humd = %v, want a non-zero exit\n%s", err, out)
	}
	if exitErr.ExitCode() != exitError {
		t.Errorf("second humd exited %d, want %d\n%s", exitErr.ExitCode(), exitError, out)
	}

	conn, err := net.Dial("unix", p.socket)
	if err != nil {
		t.Fatalf("the first daemon's socket stopped answering: %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		t.Fatalf("ping the survivor: %v", err)
	}
	var response protocol.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil || !response.OK {
		t.Errorf("survivor ping = %+v (%v), want ok", response, err)
	}

	p.cmd.Process.Signal(syscall.SIGTERM)
	waitExit(t, p.cmd, 30*time.Second)
}

func TestBinaryReclaimsAStaleSocket(t *testing.T) {
	p := startProcess(t)
	socket := p.socket

	if err := p.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}
	p.cmd.Wait()

	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("a killed daemon should leave its socket behind, got %v", err)
	}

	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}
	restarted := exec.Command(binary, "--no-audio", "--socket", socket)
	restarted.Env = append(os.Environ(), "HUM_HOME="+filepath.Dir(socket))
	if err := restarted.Start(); err != nil {
		t.Fatalf("restart over a stale socket: %v", err)
	}
	t.Cleanup(func() {
		if restarted.ProcessState == nil {
			restarted.Process.Kill()
			restarted.Wait()
		}
	})

	waitForSocket(t, socket)
	if responses := send(t, socket, protocol.Request{Command: protocol.CmdPing}); !responses[0].OK {
		t.Errorf("ping after reclaiming a stale socket = %+v", responses[0])
	}

	restarted.Process.Signal(syscall.SIGTERM)
	waitExit(t, restarted, 30*time.Second)
}

func TestServeListenerExitsCleanly(t *testing.T) {
	d, _ := testDaemon(t)
	listener := &errListener{addr: shortSocket(t), serveErr: nil}
	signals := make(chan os.Signal, 2)
	code := serve(d, listener, quietLogger(), signals)
	if code != exitOK {
		t.Errorf("serve returned %d, want %d when listener exits without error", code, exitOK)
	}
}
