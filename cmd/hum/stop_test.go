package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func stopServeShutdownAndRemove(t *testing.T) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "s.sock")
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

func TestStopWithRealDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d := startHumd(t)
	socket := d.socket

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Errorf("socket %s still exists after hum stop returned", socket)
	}
}

func TestStopNotRunning(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q, want 'not running'", stdout.String())
	}
}

func TestStopTimeoutWhenSocketPersists(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":true}`+"\n")
	socket := os.Getenv("HUM_SOCKET")

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop", "--timeout", "100ms"}, &stdout, &stderr)
	_ = await()

	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitDaemonError, stderr.String())
	}
	if !strings.Contains(stderr.String(), socket) {
		t.Errorf("stderr=%q, want socket path %q", stderr.String(), socket)
	}
}

func TestStopDaemonRejectsShutdown(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":false,"error":"shutdown not allowed"}`+"\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop"}, &stdout, &stderr)
	_ = await()

	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitDaemonError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "shutdown not allowed") {
		t.Errorf("stderr=%q, want 'shutdown not allowed'", stderr.String())
	}
}

func TestStopDefaultTimeout(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	stopServeShutdownAndRemove(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d (default 10s budget), want %d; stderr=%q", code, exitOK, stderr.String())
	}
}

func TestStopStrayOperand(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"stop", "extra"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitUsage, stderr.String())
	}
}

func TestStopUnparsableFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"stop", "--bogus"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want a usage error message")
	}
}

func TestStopUnparseableDaemonResponseExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, "not json\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"stop"}, &stdout, &stderr)
	_ = await()

	if code != exitUnreachable {
		t.Fatalf("exit %d, want %d (unreachable); stderr=%q", code, exitUnreachable, stderr.String())
	}
	if strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q: a bad response must not be reported as 'not running'", stdout.String())
	}
}

func TestStopJSONPrintsRawResponse(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	stopServeShutdownAndRemove(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"stop", "--json"}, &stdout, &stderr)

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
