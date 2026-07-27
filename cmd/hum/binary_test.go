package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The exit codes are a contract with shell scripts, so they are asserted
// against a real process. An in-process test of run cannot catch a main that
// discards run's return value, and `go run` is useless here: it reports 1 for
// any non-zero program exit, collapsing the very distinction being tested.
var buildHum = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "hum-bin")
	if err != nil {
		return "", err
	}
	binary := filepath.Join(dir, "hum")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		return "", errors.New(string(out))
	}
	return binary, nil
})

// runBinary reports the process exit code and combined output.
func runBinary(t *testing.T, socket string, args ...string) (int, string) {
	t.Helper()
	binary, err := buildHum()
	if err != nil {
		t.Fatalf("build hum: %v", err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "HUM_SOCKET="+socket)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run %v: %v", args, err)
	}
	return exitErr.ExitCode(), string(out)
}

func TestBinaryExitCodes(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.sock")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no arguments", nil, exitUsage},
		{"help", []string{"help"}, exitUsage},
		{"unknown command", []string{"bogus"}, exitUsage},
		{"no daemon", []string{"ping"}, exitUnreachable},
		{"no daemon, status", []string{"status"}, exitUnreachable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := runBinary(t, absent, tc.args...)

			if code != tc.want {
				t.Errorf("hum %v exited %d, want %d\n%s", tc.args, code, tc.want, out)
			}
		})
	}
}

// Distinguishing 3 from 1 is what lets a CI script tell "Hum is not running"
// from "the work failed", so the message must be actionable, not a dial error.
func TestBinaryReportsAnAbsentDaemonActionably(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.sock")

	code, out := runBinary(t, absent, "ping")

	if code != exitUnreachable {
		t.Fatalf("hum ping exited %d, want %d\n%s", code, exitUnreachable, out)
	}
	if !strings.Contains(out, absent) {
		t.Errorf("output = %q, want the socket path named", out)
	}
	if !strings.Contains(out, "humd") {
		t.Errorf("output = %q, want it to say how to start the daemon", out)
	}
}
