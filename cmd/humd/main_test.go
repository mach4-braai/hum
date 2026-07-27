package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunWithoutArgumentsPrintsUsageToStderrAndExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "usage:") {
		t.Errorf("stderr = %q, want it to contain %q", got, "usage:")
	}
}

func TestMainExitsWithRunCodeAndWritesUsageToStderr(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr, origExit := os.Stderr, exit
	t.Cleanup(func() { os.Stderr, exit = origStderr, origExit })
	var code int
	exited := false
	os.Stderr = w
	exit = func(c int) { code, exited = c, true }

	main()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}

	if !exited {
		t.Fatal("main returned without exiting; the process would report success")
	}
	if code != 2 {
		t.Errorf("main exit code = %d, want 2 from run", code)
	}
	if !strings.Contains(string(out), "usage:") {
		t.Errorf("main stderr = %q, want it to contain %q", out, "usage:")
	}
}
