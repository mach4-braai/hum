//go:build !windows

package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorSocketCheckStatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "nosearch")
	if err := os.Mkdir(sub, 0o000); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(sub, 0o755) })
	t.Setenv("HUM_SOCKET", filepath.Join(sub, "hum.sock"))
	c := doctorSocketCheck()
	os.Chmod(sub, 0o755)
	if c.Status != "warn" {
		t.Errorf("status = %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "hum.sock") {
		t.Errorf("detail should contain socket name; got %q", c.Detail)
	}
}

func TestStartProjectRootUnresolvableWithoutRootFlag(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	os.RemoveAll(dir)

	var stderr bytes.Buffer
	code := run([]string{"start"}, &bytes.Buffer{}, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit %d want %d; stderr=%q", code, exitDaemonError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cannot resolve the project root") {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), "cannot resolve the project root")
	}
}

func TestStopMatchesSIGTERM(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	stubUnsupervised(t)
	d1 := startHumd(t)
	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"daemon", "stop"}, &stdout1, &stderr1); code != exitOK {
		t.Fatalf("hum daemon stop exited %d; stderr=%q", code, stderr1.String())
	}
	d1.stop()
	humStopLogs := d1.logs()

	d2 := startHumd(t)
	d2.stop()
	sigtermLogs := d2.logs()

	for _, marker := range []string{"shutting down", "waiting for voices to fade", "stopped"} {
		if !strings.Contains(humStopLogs, marker) {
			t.Errorf("hum daemon stop logs missing %q\nlogs:\n%s", marker, humStopLogs)
		}
		if !strings.Contains(sigtermLogs, marker) {
			t.Errorf("SIGTERM logs missing %q\nlogs:\n%s", marker, sigtermLogs)
		}
	}
	if !strings.Contains(humStopLogs, "shutdown command") {
		t.Errorf("hum daemon stop logs missing reason 'shutdown command'\nlogs:\n%s", humStopLogs)
	}
	if !strings.Contains(sigtermLogs, "terminated") {
		t.Errorf("SIGTERM logs missing signal name 'terminated'\nlogs:\n%s", sigtermLogs)
	}
}

func stubLaunchd(t *testing.T) {
	t.Helper()
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not active")
	})
}

func TestDaemonStopSupervisedWithStaleSocketExitsZero(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := staleSocket(t, dir)
	t.Setenv("HUM_HOME", dir)
	t.Setenv("HUM_SOCKET", socket)
	stubLaunchd(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q, want 'not running' for a socket nobody is accepting on", stdout.String())
	}
	if strings.Contains(stderr.String(), "brew services restart hum") {
		t.Errorf("stderr=%q: a stale socket stranded nothing", stderr.String())
	}
}

func TestDaemonStopSupervisedWithDeniedSocketExitsThree(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "hum.sock")
	t.Cleanup(func() {
		os.Chmod(socket, 0o600)
		os.RemoveAll(dir)
	})
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if err := os.Chmod(socket, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUM_HOME", dir)
	t.Setenv("HUM_SOCKET", socket)
	stubLaunchd(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"daemon", "stop"}, &stdout, &stderr)

	if code != exitUnreachable {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitUnreachable, stderr.String())
	}
	if strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout=%q: a permission fault is not evidence the daemon is stopped", stdout.String())
	}
}
