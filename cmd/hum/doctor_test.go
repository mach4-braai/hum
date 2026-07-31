package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/config"
)

func TestDoctorStrayArgumentIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor", "extra"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitUsage, stderr.String())
	}
}

func TestDoctorNoDaemonReportsChecksAndExitsNonZero(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"doctor"}, &stdout, &stderr)

	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitDaemonError, out, stderr.String())
	}
	for _, want := range []string{"fail", "daemon", "music.root", "music.theme", "theme"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorHealthyDaemonExitsZero(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	startHumd(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"doctor"}, &stdout, &stderr)

	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, out, stderr.String())
	}
	if !strings.Contains(out, "warn") {
		t.Errorf("expected at least one warn (nop renderer) but got:\n%s", out)
	}
	if strings.Contains(out, "fail") {
		t.Errorf("unexpected fail in output:\n%s", out)
	}
}

func TestDoctorConfigProvenance(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	unreachableSocket(t)

	projDir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	humDir := filepath.Join(projDir, ".hum")
	if err := os.MkdirAll(humDir, 0o700); err != nil {
		t.Fatalf("mkdir .hum: %v", err)
	}
	projectConfig := "music:\n  root: A\n"
	if err := os.WriteFile(filepath.Join(humDir, "config.yaml"), []byte(projectConfig), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	run([]string{"doctor"}, &stdout, &stderr)
	out := stdout.String()

	if !strings.Contains(out, "layer: project") {
		t.Errorf("expected music.root to show project layer; got:\n%s", out)
	}
	if !strings.Contains(out, "layer: default") {
		t.Errorf("expected some field to show default layer; got:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "music.root") && strings.Contains(line, "A") {
			if !strings.Contains(line, "project") {
				t.Errorf("music.root row does not show project layer: %q", line)
			}
			return
		}
	}
	t.Errorf("could not find music.root row with value A in output:\n%s", out)
}

func TestDoctorVersionMismatchIsWarn(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	statusPayload := fmt.Sprintf(
		`{"ok":true,"data":{"version":"v99.0.0","renderer":"nop","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`,
	)
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
	)

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)

	await()
	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, out, stderr.String())
	}
	if !strings.Contains(out, "warn") {
		t.Errorf("expected warn for version mismatch; got:\n%s", out)
	}
	if !strings.Contains(out, "v99.0.0") {
		t.Errorf("expected daemon version in output; got:\n%s", out)
	}
	if strings.Contains(out, "fail") {
		t.Errorf("version mismatch must not be a fail; got:\n%s", out)
	}
}

func TestDoctorAudioTestNotPlayedNopRenderer(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	statusPayload := `{"ok":true,"data":{"version":"dev","renderer":"nop","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":0}}`
	audioPayload := `{"ok":true,"data":{"played":false,"renderer":"nop","muted":false,"seconds":0}}`
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		audioPayload+"\n",
	)

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)

	await()
	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (warn must not fail)\nstdout:\n%s\nstderr:\n%s", code, exitOK, out, stderr.String())
	}
	if !strings.Contains(out, "nop") {
		t.Errorf("expected nop renderer mentioned in no-op message; got:\n%s", out)
	}
	if !strings.Contains(out, "no-op") {
		t.Errorf("expected no-op message; got:\n%s", out)
	}
}

func TestDoctorAudioTestNotPlayedMuted(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	statusPayload := `{"ok":true,"data":{"version":"dev","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":true,"sessions":[],"sample_rate":44100}}`
	audioPayload := `{"ok":true,"data":{"played":false,"renderer":"audio","muted":true,"seconds":0}}`
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		audioPayload+"\n",
	)

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)

	await()
	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (muted no-op must not fail)\nstdout:\n%s\nstderr:\n%s", code, exitOK, out, stderr.String())
	}
	if !strings.Contains(out, "muted") {
		t.Errorf("expected muted mentioned in no-op message; got:\n%s", out)
	}
}

func TestDoctorAudioTestPlayed(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	statusPayload := `{"ok":true,"data":{"version":"dev","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`
	audioPayload := `{"ok":true,"data":{"played":true,"renderer":"audio","muted":false,"seconds":2.0}}`
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		audioPayload+"\n",
	)

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)

	await()
	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, out, stderr.String())
	}
	if !strings.Contains(out, "tone played") {
		t.Errorf("expected tone played in output; got:\n%s", out)
	}
}

func TestDoctorAudioTestNoDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)

	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitDaemonError, out, stderr.String())
	}
	if !strings.Contains(out, "audio-test") {
		t.Errorf("expected audio-test row in output; got:\n%s", out)
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	run([]string{"doctor", "--json"}, &stdout, &stderr)

	var entries []map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &entries); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %q", err, stdout.String())
	}
	if len(entries) == 0 {
		t.Fatal("JSON output is empty")
	}
	for i, entry := range entries {
		for _, field := range []string{"status", "name", "detail"} {
			if _, ok := entry[field]; !ok {
				t.Errorf("entry %d missing field %q: %v", i, field, entry)
			}
		}
	}
	doctorAssertJSONHasName(t, entries, "client")
	doctorAssertJSONHasName(t, entries, "daemon")
	doctorAssertJSONHasName(t, entries, "theme")
}

func doctorAssertJSONHasName(t *testing.T, entries []map[string]string, name string) {
	t.Helper()
	for _, e := range entries {
		if e["name"] == name {
			return
		}
	}
	t.Errorf("JSON output missing entry with name %q", name)
}

func TestDoctorInvalidThemeFails(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	unreachableSocket(t)

	themesDir := filepath.Join(humHome, "themes")
	if err := os.MkdirAll(themesDir, 0o700); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "broken.yaml"), []byte("invalid: {unclosed\n"), 0o600); err != nil {
		t.Fatalf("write bad theme: %v", err)
	}

	globalConfig := "music:\n  theme: broken\n"
	if err := os.WriteFile(filepath.Join(humHome, "config.yaml"), []byte(globalConfig), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)

	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d (invalid theme must fail)\nstdout:\n%s\nstderr:\n%s", code, exitDaemonError, out, stderr.String())
	}

	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "theme") && strings.Contains(line, "fail") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a fail row for theme; got:\n%s", out)
	}
}

func TestDoctorBogusFlag(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor", "--bogus"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitUsage, stderr.String())
	}
}

func TestDoctorPingReturnsFalse(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":false,"error":"bad ping"}`+"\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "daemon") && strings.Contains(line, "fail") && strings.Contains(line, "bad ping") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected fail row for daemon with 'bad ping'; got:\n%s", out)
	}
}

func TestDoctorPingNonDaemonQueryError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, "not-valid-json\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "daemon") && strings.Contains(line, "fail") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected fail row for daemon; got:\n%s", out)
	}
}

func TestDoctorStatusFails(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		`{"ok":false,"error":"status error"}`+"\n",
	)
	var stdout, stderr bytes.Buffer
	run([]string{"doctor"}, &stdout, &stderr)
	await()
	out := stdout.String()
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "versions") && strings.Contains(line, "warn") && strings.Contains(line, "could not fetch") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected versions warn 'could not fetch'; got:\n%s", out)
	}
}

func TestDoctorStatusUndecodable(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		`{"ok":true,"data":{"sessions":"many"}}`+"\n",
	)
	var stdout, stderr bytes.Buffer
	run([]string{"doctor"}, &stdout, &stderr)
	await()
	out := stdout.String()
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "versions") && strings.Contains(line, "warn") && strings.Contains(line, "could not fetch") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected versions warn 'could not fetch' for undecodable status; got:\n%s", out)
	}
}

func TestDoctorInvalidGlobalConfig(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	unreachableSocket(t)
	if err := os.WriteFile(filepath.Join(humHome, "config.yaml"), []byte("music:\n  scale: klingon\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	configFail := false
	themeUnknown := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "config") && strings.Contains(line, "fail") {
			configFail = true
		}
		if strings.Contains(line, "theme") && strings.Contains(line, "warn") && strings.Contains(line, "unknown") {
			themeUnknown = true
		}
	}
	if !configFail {
		t.Errorf("expected fail row for config; got:\n%s", out)
	}
	if !themeUnknown {
		t.Errorf("expected theme warn with 'unknown'; got:\n%s", out)
	}
}

func TestDoctorUserTheme(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	unreachableSocket(t)
	themesDir := filepath.Join(humHome, "themes")
	if err := os.MkdirAll(themesDir, 0o700); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	userTheme := strings.Join([]string{
		"name: mytest",
		"waveform: sine",
		"drone:",
		"  attack: 2.5",
		"  release: 3.0",
		"  gain: 0.5",
		"  harmonic: 0.15",
		"  tremolo_hz: 5.0",
		"  detune_cents: 8.0",
		"phrases:",
		"  completion_octaves: 2",
		"  completion_duration: 0.2",
		"  completion_gain: 0.7",
		"  failure_interval: -3",
		"  failure_duration: 1.2",
		"  failure_gain: 0.35",
		"  cancelled_sounds: false",
		"  cancelled_duration: 0.4",
		"  cancelled_gain: 0.3",
		"  attack: 0.02",
		"  decay: 0.15",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(themesDir, "mytest.yaml"), []byte(userTheme), 0o600); err != nil {
		t.Fatalf("write user theme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(humHome, "config.yaml"), []byte("music:\n  theme: mytest\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var stdout, stderr bytes.Buffer
	run([]string{"doctor"}, &stdout, &stderr)
	out := stdout.String()
	userPath := filepath.Join(humHome, "themes", "mytest.yaml")
	if !strings.Contains(out, "user:") || !strings.Contains(out, userPath) {
		t.Errorf("expected user theme path %s in output; got:\n%s", userPath, out)
	}
}

func TestDoctorMusicCheckInvalidRoot(t *testing.T) {
	cfg := &config.Config{Music: config.MusicConfig{Root: "Z#", Scale: "minor_pentatonic"}}
	c := doctorMusicCheck(cfg)
	if c.Status != "fail" {
		t.Errorf("status = %q, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "root") || !strings.Contains(c.Detail, "Z#") {
		t.Errorf("detail should name offending root; got %q", c.Detail)
	}
}

func TestDoctorMusicCheckInvalidScale(t *testing.T) {
	cfg := &config.Config{Music: config.MusicConfig{Root: "D", Scale: "klingon"}}
	c := doctorMusicCheck(cfg)
	if c.Status != "fail" {
		t.Errorf("status = %q, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "scale") || !strings.Contains(c.Detail, "klingon") {
		t.Errorf("detail should name offending scale; got %q", c.Detail)
	}
}

func TestDoctorReportsOctaveWithItsLayer(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	unreachableSocket(t)
	if err := os.WriteFile(filepath.Join(humHome, "config.yaml"), []byte("music:\n  octave: 5\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	run([]string{"doctor"}, &stdout, &stderr)
	out := stdout.String()

	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "music.octave") {
			continue
		}
		if !strings.Contains(line, "5") || !strings.Contains(line, "layer: global") {
			t.Errorf("music.octave row does not report 5 from the global layer: %q", line)
		}
		return
	}
	t.Errorf("no music.octave row in output:\n%s", out)
}

func TestDoctorMusicCheckNamesTheSoundingPitch(t *testing.T) {
	cfg := &config.Config{Music: config.MusicConfig{Root: "D", Octave: 5, Scale: "minor_pentatonic"}}
	c := doctorMusicCheck(cfg)
	if c.Status != "pass" {
		t.Fatalf("status = %q, want pass", c.Status)
	}
	if !strings.Contains(c.Detail, "root D5") {
		t.Errorf("detail should name the sounding pitch; got %q", c.Detail)
	}
}

func TestDoctorSocketCheckNotASocket(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notsock")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	t.Setenv("HUM_SOCKET", f.Name())
	c := doctorSocketCheck()
	if c.Status != "fail" {
		t.Errorf("status = %q, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "not a socket") {
		t.Errorf("detail should say 'not a socket'; got %q", c.Detail)
	}
}

func TestDoctorAudioTestTransportError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusPayload := fmt.Sprintf(
		`{"ok":true,"data":{"version":"%s","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`,
		version,
	)
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
	)
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "audio-test") && strings.Contains(line, "fail") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected fail row for audio-test; got:\n%s", out)
	}
}

func TestDoctorAudioTestDaemonReturnsError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusPayload := fmt.Sprintf(
		`{"ok":true,"data":{"version":"%s","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`,
		version,
	)
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		`{"ok":false,"error":"device error"}`+"\n",
	)
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "audio-test") && strings.Contains(line, "fail") && strings.Contains(line, "device error") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audio-test fail with 'device error'; got:\n%s", out)
	}
}

func TestDoctorAudioTestUndecodablePayload(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusPayload := fmt.Sprintf(
		`{"ok":true,"data":{"version":"%s","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`,
		version,
	)
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		`{"ok":true,"data":"not-an-object"}`+"\n",
	)
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitDaemonError {
		t.Fatalf("exit = %d, want %d\nstdout:\n%s", code, exitDaemonError, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "audio-test") && strings.Contains(line, "fail") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audio-test fail for malformed payload; got:\n%s", out)
	}
}

func TestDoctorAudioTestNotPlayedNoOp(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusPayload := fmt.Sprintf(
		`{"ok":true,"data":{"version":"%s","renderer":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":44100}}`,
		version,
	)
	audioPayload := `{"ok":true,"data":{"played":false,"renderer":"audio","muted":false,"seconds":0}}`
	await := serveResponses(t,
		`{"ok":true}`+"\n",
		statusPayload+"\n",
		audioPayload+"\n",
	)
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--audio-test"}, &stdout, &stderr)
	await()
	out := stdout.String()
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (warn must not fail)\nstdout:\n%s", code, exitOK, out)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "audio-test") && strings.Contains(line, "warn") && strings.Contains(line, "no-op") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audio-test warn with 'no-op'; got:\n%s", out)
	}
}

func doctorRowFor(t *testing.T, out, name string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return line
		}
	}
	t.Fatalf("no %q row in doctor output:\n%s", name, out)
	return ""
}

func TestDoctorAudioRowSeparatesHeadlessFromFallback(t *testing.T) {
	cases := []struct {
		name      string
		requested string
		want      string
		unwanted  string
	}{
		{"headless by request", "nop", "headless by request", "no audio device"},
		{"device fallback", "audio", "fell back from audio", "headless"},
		{"daemon says nothing", "", "did not say what it asked for", "headless"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HUM_HOME", t.TempDir())
			status := fmt.Sprintf(
				`{"ok":true,"data":{"version":"%s","renderer":"nop","renderer_requested":"%s","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":48000}}`,
				version, tc.requested,
			)
			serveResponses(t, `{"ok":true}`+"\n", status+"\n")
			var stdout, stderr bytes.Buffer

			code := run([]string{"doctor"}, &stdout, &stderr)

			if code != exitOK {
				t.Fatalf("exit %d, want %d: a silent renderer is a warning, not a failure; stdout=%s", code, exitOK, stdout.String())
			}
			row := doctorRowFor(t, stdout.String(), "audio")
			if !strings.HasPrefix(row, "warn") {
				t.Errorf("audio row = %q, want a warning", row)
			}
			if !strings.Contains(row, tc.want) {
				t.Errorf("audio row = %q, want it to mention %q", row, tc.want)
			}
			if strings.Contains(row, tc.unwanted) {
				t.Errorf("audio row = %q, must not claim %q", row, tc.unwanted)
			}
		})
	}
}

func TestDoctorAudioRowPassesForARealDevice(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	status := fmt.Sprintf(
		`{"ok":true,"data":{"version":"%s","renderer":"audio","renderer_requested":"audio","scale":"minor_pentatonic","root":"D","theme":"minimal","volume":0.6,"muted":false,"sessions":[],"sample_rate":48000}}`,
		version,
	)
	serveResponses(t, `{"ok":true}`+"\n", status+"\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"doctor"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, want %d; stdout=%s", code, exitOK, stdout.String())
	}

	row := doctorRowFor(t, stdout.String(), "audio")
	if !strings.HasPrefix(row, "pass") {
		t.Errorf("audio row = %q, want a pass when the requested renderer is the one running", row)
	}
	if !strings.Contains(row, "sample_rate=48000") {
		t.Errorf("audio row = %q, want the sample rate reported", row)
	}
}
