package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

var buildHum = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "hum-bin")
	if err != nil {
		return "", err
	}
	binary := filepath.Join(dir, "hum"+exeSuffix)
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		return "", errors.New(string(out))
	}
	return binary, nil
})

func runBinary(t *testing.T, socket string, args ...string) (int, string) {
	t.Helper()
	return runBinaryWith(t, socket, nil, args...)
}

func runBinaryWith(t *testing.T, socket string, extra []string, args ...string) (int, string) {
	t.Helper()
	binary, err := buildHum()
	if err != nil {
		t.Fatalf("build hum: %v", err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(append(os.Environ(), "HUM_SOCKET="+socket), extra...)
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
