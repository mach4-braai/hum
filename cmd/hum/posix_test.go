//go:build !windows

package main

import (
	"bytes"
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

	d1 := startHumd(t)
	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"stop"}, &stdout1, &stderr1); code != exitOK {
		t.Fatalf("hum stop exited %d; stderr=%q", code, stderr1.String())
	}
	d1.stop()
	humStopLogs := d1.logs()

	d2 := startHumd(t)
	d2.stop()
	sigtermLogs := d2.logs()

	for _, marker := range []string{"shutting down", "waiting for voices to fade", "stopped"} {
		if !strings.Contains(humStopLogs, marker) {
			t.Errorf("hum stop logs missing %q\nlogs:\n%s", marker, humStopLogs)
		}
		if !strings.Contains(sigtermLogs, marker) {
			t.Errorf("SIGTERM logs missing %q\nlogs:\n%s", marker, sigtermLogs)
		}
	}
	if !strings.Contains(humStopLogs, "shutdown command") {
		t.Errorf("hum stop logs missing reason 'shutdown command'\nlogs:\n%s", humStopLogs)
	}
	if !strings.Contains(sigtermLogs, "terminated") {
		t.Errorf("SIGTERM logs missing signal name 'terminated'\nlogs:\n%s", sigtermLogs)
	}
}
