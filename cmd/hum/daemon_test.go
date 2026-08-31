package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func daemonStopServeShutdownAndRemove(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hum")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	t.Setenv("HUM_SOCKET", socket)
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close(); os.Remove(socket) })
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		var req protocol.Request
		_ = json.NewDecoder(conn).Decode(&req)
		_, _ = io.WriteString(conn, `{"ok":true}`+"\n")
		conn.Close()
		os.Remove(socket)
		l.Close()
	}()
}

func stubUnsupervised(t *testing.T) {
	t.Helper()
	stubProbe(t, func(_ string) error { return errors.New("not active") })
}

func countPayloadRequests(reqs []protocol.Request) []protocol.Request {
	var out []protocol.Request
	for _, r := range reqs {
		if r.Command != "" || r.Event != nil {
			out = append(out, r)
		}
	}
	return out
}

func TestDaemonSubcommandRequired(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "stop") {
		t.Errorf("stderr=%q, want 'stop' in usage hint", stderr.String())
	}
}

func TestDaemonUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "quit"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "quit") {
		t.Errorf("stderr=%q, want the unknown subcommand name", stderr.String())
	}
}

func TestDaemonUnknownFlagBeforeSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "--bogus", "stop"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
}

func TestDaemonStopNotRunning(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q, want 'not running'", stdout.String())
	}
}

func TestDaemonStopWithRealDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d := startHumd(t)
	socket := d.socket
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Errorf("socket %s still exists after daemon stop returned", socket)
	}
}

func TestDaemonStopTimeoutWhenSocketPersists(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":true}`+"\n")
	socket := os.Getenv("HUM_SOCKET")
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop", "--timeout", "100ms"}, &stdout, &stderr)
	_ = await()

	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitDaemonError, stderr.String())
	}
	if !strings.Contains(stderr.String(), socket) {
		t.Errorf("stderr=%q, want socket path %q", stderr.String(), socket)
	}
}

func TestDaemonStopDaemonRejectsShutdown(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":false,"error":"shutdown not allowed"}`+"\n")
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)
	_ = await()

	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitDaemonError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "shutdown not allowed") {
		t.Errorf("stderr=%q, want 'shutdown not allowed'", stderr.String())
	}
}

func TestDaemonStopDefaultTimeout(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	daemonStopServeShutdownAndRemove(t)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d (default 10s budget), want %d; stderr=%q", code, exitOK, stderr.String())
	}
}

func TestDaemonStopJSONPrintsRawResponse(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	daemonStopServeShutdownAndRemove(t)
	stubUnsupervised(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"daemon", "stop", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Errorf("stdout=%q; --json should print the raw response envelope", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr=%q, want empty", stderr.String())
	}
}

func TestDaemonStopStrayOperand(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop", "extra"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitUsage, stderr.String())
	}
}

func TestDaemonStopUnparsableFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"daemon", "stop", "--bogus"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want a usage error message")
	}
}

func TestDaemonStopUnparseableDaemonResponseExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, "not json\n")
	stubUnsupervised(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"daemon", "stop"}, &stdout, &stderr)
	_ = await()

	if code != exitUnreachable {
		t.Fatalf("exit %d, want %d (unreachable); stderr=%q", code, exitUnreachable, stderr.String())
	}
	if strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q: a bad response must not be reported as 'not running'", stdout.String())
	}
}

func TestDaemonStopLaunchdWithoutForceExitsTwoSendsNothing(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t, `{"ok":true}`+"\n")
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not active")
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)
	reqs := await()

	if code != exitUsage {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitUsage, stderr.String())
	}
	if sent := countPayloadRequests(reqs); len(sent) != 0 {
		t.Errorf("guard sent %v, want no requests", sent)
	}
	if !strings.Contains(stderr.String(), "brew services restart hum") {
		t.Errorf("stderr=%q, want launchd restart command", stderr.String())
	}
}

func TestDaemonStopSystemdWithoutForceExitsTwoSendsNothing(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t, `{"ok":true}`+"\n")
	stubProbe(t, func(name string) error {
		if name == "systemctl" {
			return nil
		}
		return errors.New("not active")
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)
	reqs := await()

	if code != exitUsage {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitUsage, stderr.String())
	}
	if sent := countPayloadRequests(reqs); len(sent) != 0 {
		t.Errorf("guard sent %v, want no requests", sent)
	}
	if !strings.Contains(stderr.String(), "systemctl --user restart humd") {
		t.Errorf("stderr=%q, want systemd restart command", stderr.String())
	}
}

func TestDaemonStopSupervisedWithNoDaemonExitsZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUM_HOME", dir)
	t.Setenv("HUM_SOCKET", filepath.Join(dir, "absent.sock"))
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not active")
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q, want 'not running'", stdout.String())
	}
	if strings.Contains(stderr.String(), "brew services restart hum") {
		t.Errorf("stderr=%q: nothing was stranded, so no restart command belongs here", stderr.String())
	}
}

func TestDaemonStopLaunchdWithForceShutsDownAndPrintsRestartCommand(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	daemonStopServeShutdownAndRemove(t)
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not active")
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop", "--force"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "brew services restart hum") {
		t.Errorf("stdout=%q, want launchd restart command on success", stdout.String())
	}
}

func TestDaemonStopForceWithNoSupervisorWorks(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	daemonStopServeShutdownAndRemove(t)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop", "--force"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if strings.Contains(stdout.String(), "restart") {
		t.Errorf("stdout=%q, want no restart command when not supervised", stdout.String())
	}
}

func TestDaemonStopSupervisedNotRunningExitsZero(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not active")
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop", "--force"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q, want 'not running'", stdout.String())
	}
}
