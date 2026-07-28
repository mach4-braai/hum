package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionFlagPrintsToStdoutAndSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("stdout = %q, want it to contain the version %q", stdout.String(), version)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestUnexpectedArgumentIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"start"}, &stdout, &stderr)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if got := stderr.String(); !strings.Contains(got, "start") || !strings.Contains(got, "usage:") {
		t.Errorf("stderr = %q, want it to name the argument and print usage", got)
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--nonsense"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}

func TestUnknownLogLevelIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--log-level", "chatty"}, &stdout, &stderr)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if got := stderr.String(); !strings.Contains(got, "chatty") {
		t.Errorf("stderr = %q, want it to name the rejected level", got)
	}
}

func TestParseLevel(t *testing.T) {
	for _, name := range []string{"debug", "info", "warn", "error"} {
		if _, err := parseLevel(name); err != nil {
			t.Errorf("parseLevel(%q) = %v, want nil", name, err)
		}
	}
	if _, err := parseLevel("trace"); err == nil {
		t.Error("parseLevel(\"trace\") = nil, want an error")
	}
}

func TestUnreadableThemeStopsStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(home+"/config.yaml", []byte("music:\n  theme: nonexistent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--no-audio"}, &stdout, &stderr); code != exitError {
		t.Errorf("exit code = %d, want %d when the theme cannot load", code, exitError)
	}
	if got := stderr.String(); !strings.Contains(got, "nonexistent") {
		t.Errorf("stderr = %q, want it to name the missing theme", got)
	}
}

func TestInvalidConfigStopsStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(home+"/config.yaml", []byte("music:\n  scale: klingon\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--no-audio"}, &stdout, &stderr); code != exitError {
		t.Errorf("exit code = %d, want %d when config is invalid", code, exitError)
	}
}

func TestUnknownRendererStopsStartup(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"--renderer", "hue", "--socket", shortSocket(t)}, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if got := stderr.String(); !strings.Contains(got, "hue") {
		t.Errorf("stderr = %q, want it to name the unknown renderer", got)
	}
}

func TestRelativeSocketStopsStartup(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := run([]string{"--no-audio", "--socket", "relative.sock"}, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if got := stderr.String(); !strings.Contains(got, "absolute") {
		t.Errorf("stderr = %q, want it to explain that the socket path must be absolute", got)
	}
}

func TestMainExitsWithRunCode(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout, origArgs, origExit := os.Stdout, os.Args, exit
	t.Cleanup(func() { os.Stdout, os.Args, exit = origStdout, origArgs, origExit })

	var code int
	exited := false
	os.Stdout = w
	os.Args = []string{"humd", "--version"}
	exit = func(c int) { code, exited = c, true }

	main()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}

	if !exited {
		t.Fatal("main returned without exiting; the process would report success regardless of run")
	}
	if code != exitOK {
		t.Errorf("main exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(string(out), "humd") {
		t.Errorf("main stdout = %q, want the version line", out)
	}
}
