//go:build !windows

package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
)

func staleSocket(t *testing.T, dir string) string {
	t.Helper()
	socket := dir + "/hum.sock"
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()
	return socket
}

func TestDialErrMissingSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := dir + "/hum.sock"
	t.Setenv("HUM_SOCKET", socket)
	t.Setenv("HUM_HOME", dir)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"ping"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("ping exit = %d, want %d", code, exitUnreachable)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "no daemon listening at") {
		t.Errorf("stderr = %q, want 'no daemon listening at'", msg)
	}
	if !strings.Contains(msg, socket) {
		t.Errorf("stderr = %q, want socket path %q", msg, socket)
	}
	if !strings.Contains(msg, "humd") {
		t.Errorf("stderr = %q, want daemon-start hint", msg)
	}

	var stdout2, stderr2 bytes.Buffer
	code2 := run([]string{"daemon", "stop"}, &stdout2, &stderr2)
	if code2 != exitOK {
		t.Errorf("daemon stop exit = %d, want %d; stderr=%q", code2, exitOK, stderr2.String())
	}
	if !strings.Contains(stdout2.String(), "not running") {
		t.Errorf("daemon stop stdout = %q, want 'not running'", stdout2.String())
	}

	var stdout3, stderr3 bytes.Buffer
	run([]string{"doctor"}, &stdout3, &stderr3)
	row := doctorRowFor(t, stdout3.String(), "daemon")
	if !strings.Contains(row, "fail") {
		t.Errorf("daemon row = %q, want fail", row)
	}
	if !strings.Contains(row, socket) {
		t.Errorf("daemon row = %q, want socket path", row)
	}
}

func TestDialErrStaleSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := staleSocket(t, dir)
	t.Setenv("HUM_SOCKET", socket)
	t.Setenv("HUM_HOME", dir)
	stubUnsupervised(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"ping"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("ping exit = %d, want %d", code, exitUnreachable)
	}
	msg := stderr.String()
	if !strings.Contains(msg, socket) {
		t.Errorf("stderr = %q, want socket path %q", msg, socket)
	}
	if !strings.Contains(msg, "nothing is accepting") {
		t.Errorf("stderr = %q, want 'nothing is accepting'", msg)
	}

	os.Remove(socket)
	socket2 := staleSocket(t, dir)
	if err := os.Rename(socket2, socket); err != nil {
		t.Fatal(err)
	}

	var stdout2, stderr2 bytes.Buffer
	code2 := run([]string{"daemon", "stop"}, &stdout2, &stderr2)
	os.Remove(socket)
	if code2 != exitOK {
		t.Errorf("daemon stop exit = %d, want %d; stderr=%q", code2, exitOK, stderr2.String())
	}
	if !strings.Contains(stdout2.String(), "not running") {
		t.Errorf("daemon stop stdout = %q, want 'not running'", stdout2.String())
	}

	socket3 := staleSocket(t, dir)
	if err := os.Rename(socket3, socket); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUM_SOCKET", socket)

	var stdout3, stderr3 bytes.Buffer
	run([]string{"doctor"}, &stdout3, &stderr3)
	os.Remove(socket)
	row := doctorRowFor(t, stdout3.String(), "daemon")
	if !strings.Contains(row, "fail") {
		t.Errorf("daemon row = %q, want fail", row)
	}
	if !strings.Contains(row, "nothing is accepting") {
		t.Errorf("daemon row = %q, want 'nothing is accepting'", row)
	}
}

func TestDialErrDeniedSocket(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	socket := dir + "/hum.sock"
	t.Cleanup(func() {
		os.Chmod(socket, 0o600)
		os.RemoveAll(dir)
	})
	t.Setenv("HUM_SOCKET", socket)
	t.Setenv("HUM_HOME", dir)
	stubUnsupervised(t)

	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()
	if err := os.Chmod(socket, 0o000); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"ping"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("ping exit = %d, want %d", code, exitUnreachable)
	}
	msg := stderr.String()
	if strings.Contains(msg, "start it with") {
		t.Errorf("stderr should not contain daemon-start hint; got %q", msg)
	}
	if strings.Contains(msg, "`humd`") {
		t.Errorf("stderr should not name humd binary; got %q", msg)
	}
	if !strings.Contains(msg, "hum doctor") {
		t.Errorf("stderr should mention 'hum doctor'; got %q", msg)
	}
	if !strings.Contains(msg, "uid") {
		t.Errorf("stderr should contain uid info; got %q", msg)
	}

	var stdout2, stderr2 bytes.Buffer
	code2 := run([]string{"daemon", "stop"}, &stdout2, &stderr2)
	if code2 != exitUnreachable {
		t.Errorf("daemon stop exit = %d, want %d; stderr=%q", code2, exitUnreachable, stderr2.String())
	}

	var stdout3, stderr3 bytes.Buffer
	run([]string{"doctor"}, &stdout3, &stderr3)
	row := doctorRowFor(t, stdout3.String(), "daemon")
	if !strings.Contains(row, "fail") {
		t.Errorf("daemon row = %q, want fail", row)
	}
	if !strings.Contains(row, "uid") {
		t.Errorf("daemon row = %q, want uid info", row)
	}
}

func TestClassifyDialErrorDefault(t *testing.T) {
	err := classifyDialError(errors.New("connection timed out"), "/tmp/hum.sock")
	if !strings.Contains(err.Error(), "--timeout") {
		t.Errorf("got %q, want '--timeout' in message", err.Error())
	}
	if errors.Is(err, errSocketDenied) {
		t.Error("timeout error should not be errSocketDenied")
	}
	if !errors.Is(err, errNoDaemon) {
		t.Error("timeout error should wrap errNoDaemon")
	}
}

func TestDeniedErrorStatFails(t *testing.T) {
	err := deniedError("/nonexistent/directory/hum.sock")
	if !errors.Is(err, errSocketDenied) {
		t.Errorf("want errSocketDenied, got %v", err)
	}
	if !strings.Contains(err.Error(), "uid") {
		t.Errorf("want uid in message, got %q", err.Error())
	}
}
