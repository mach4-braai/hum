package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

const themeListResp = `{"ok":true,"data":{"themes":["minimal","orchestra"],"active":"minimal"}}` + "\n"
const themeUseResp = `{"ok":true,"data":{"theme":"minimal"}}` + "\n"

func TestThemeListShowsThemesWithActiveMark(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	serveOne(t, themeListResp)
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "list"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "* minimal") {
		t.Errorf("stdout = %q, want active theme marked with *", out)
	}
	if !strings.Contains(out, "orchestra") {
		t.Errorf("stdout = %q, want orchestra listed", out)
	}
	if strings.Contains(out, "* orchestra") {
		t.Errorf("stdout = %q, orchestra must not be marked active", out)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestThemeListJSON(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	serveOne(t, themeListResp)
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "list", "--json"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	var p protocol.ThemeListPayload
	if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
		t.Fatalf("stdout = %q, want JSON ThemeListPayload: %v", stdout.String(), err)
	}
	if p.Active != "minimal" || len(p.Themes) != 2 {
		t.Errorf("payload = %+v, want minimal active with 2 themes", p)
	}
	if strings.Contains(stdout.String(), "* ") {
		t.Errorf("stdout = %q, want no decorated list under --json", stdout.String())
	}
}

func TestThemeListDaemonFailure(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	serveOne(t, `{"ok":false,"error":"theme engine not ready"}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "list"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit = %d, want %d", code, exitDaemonError)
	}
	if !strings.Contains(stderr.String(), "theme engine not ready") {
		t.Errorf("stderr = %q, want daemon error message", stderr.String())
	}
}

func TestThemeUse(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, themeUseResp)
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "use", "minimal"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	got := await()
	if got.Command != protocol.CmdThemeUse {
		t.Errorf("daemon received command %q, want %q", got.Command, protocol.CmdThemeUse)
	}
	if got.Value != "minimal" {
		t.Errorf("daemon received value %q, want \"minimal\"", got.Value)
	}
	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg == nil || cfg.Music.Theme != "minimal" {
		theme := "<absent>"
		if cfg != nil {
			theme = cfg.Music.Theme
		}
		t.Errorf("music.theme = %q, want \"minimal\"", theme)
	}
}

func TestThemeUseIdempotent(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t, themeUseResp, themeUseResp)
	var b1, e1, b2, e2 bytes.Buffer

	code1 := run([]string{"theme", "use", "minimal"}, &b1, &e1)
	code2 := run([]string{"theme", "use", "minimal"}, &b2, &e2)

	if code1 != exitOK {
		t.Fatalf("first run exit = %d, want 0; stderr = %q", code1, e1.String())
	}
	if code2 != exitOK {
		t.Fatalf("second run exit = %d, want 0; stderr = %q", code2, e2.String())
	}
	if reqs := await(); len(reqs) != 2 {
		t.Errorf("daemon received %d requests, want 2", len(reqs))
	}
	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg == nil || cfg.Music.Theme != "minimal" {
		t.Errorf("music.theme = %q after second run, want \"minimal\"", cfg.Music.Theme)
	}
}

func TestThemeUseBogusExitsOneWithMessage(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "use", "bogus"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit = %d, want %d (exitDaemonError)", code, exitDaemonError)
	}
	if msg := stderr.String(); !strings.Contains(msg, "minimal") {
		t.Errorf("stderr = %q, want valid theme name in message", msg)
	}
	if reqs := await(); len(reqs) != 0 {
		t.Errorf("daemon received %d requests, want 0 (client-side exit)", len(reqs))
	}
	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg != nil {
		t.Errorf("config was written after bogus theme, want file absent")
	}
}

func TestThemeUseDaemonRejection(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	serveOne(t, `{"ok":false,"error":"theme swap failed"}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "use", "minimal"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit = %d, want %d", code, exitDaemonError)
	}
	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg != nil {
		t.Errorf("config was written despite daemon rejection, want absent")
	}
}

func TestThemeUsePersistFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("- not a mapping\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	serveOne(t, themeUseResp)
	var stdout, stderr bytes.Buffer

	code := run([]string{"theme", "use", "minimal"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit = %d, want %d", code, exitDaemonError)
	}
}

func TestThemeFlagsInterleave(t *testing.T) {
	listCases := [][]string{
		{"theme", "--json", "list"},
		{"theme", "list", "--json"},
	}
	for _, args := range listCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("HUM_HOME", t.TempDir())
			serveOne(t, themeListResp)
			var stdout, stderr bytes.Buffer
			code := run(args, &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("exit = %d; stderr = %q", code, stderr.String())
			}
			var p protocol.ThemeListPayload
			if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
				t.Errorf("stdout = %q, want JSON ThemeListPayload: %v", stdout.String(), err)
			}
		})
	}

	useCases := [][]string{
		{"theme", "use", "minimal", "--json"},
		{"theme", "--json", "use", "minimal"},
		{"theme", "use", "--json", "minimal"},
	}
	for _, args := range useCases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("HUM_HOME", t.TempDir())
			serveOne(t, themeUseResp)
			var stdout, stderr bytes.Buffer
			code := run(args, &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("exit = %d; stderr = %q", code, stderr.String())
			}
			var p protocol.ThemeUsePayload
			if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
				t.Errorf("stdout = %q, want JSON ThemeUsePayload: %v", stdout.String(), err)
			}
		})
	}
}

func TestThemeUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no subcommand", []string{"theme"}},
		{"unknown subcommand", []string{"theme", "restyle"}},
		{"list extra arg", []string{"theme", "list", "extra"}},
		{"use no name", []string{"theme", "use"}},
		{"use two names", []string{"theme", "use", "a", "b"}},
		{"bad flag", []string{"theme", "--badFlag", "list"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HUM_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d (exitUsage); stderr = %q", code, exitUsage, stderr.String())
			}
			if stderr.Len() == 0 {
				t.Error("stderr empty, want a usage message")
			}
		})
	}
}

func TestThemeAbsentDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var b1, e1, b2, e2 bytes.Buffer

	code1 := run([]string{"theme", "list"}, &b1, &e1)
	if code1 != exitUnreachable {
		t.Errorf("list absent daemon: exit = %d, want %d", code1, exitUnreachable)
	}

	code2 := run([]string{"theme", "use", "minimal"}, &b2, &e2)
	if code2 != exitUnreachable {
		t.Errorf("use absent daemon: exit = %d, want %d", code2, exitUnreachable)
	}
}
