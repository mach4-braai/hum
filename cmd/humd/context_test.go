package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

func project(t *testing.T, body string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "proj")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	hum := filepath.Join(dir, paths.ProjectDirName)
	if err := os.MkdirAll(hum, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hum, paths.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("canonicalise %s: %v", dir, err)
	}
	return canonical
}

func statusOf(t *testing.T, socket string) protocol.StatusPayload {
	t.Helper()

	responses := send(t, socket, protocol.Request{Command: protocol.CmdStatus})
	if !responses[0].OK {
		t.Fatalf("status = %+v", responses[0])
	}
	var status protocol.StatusPayload
	if err := json.Unmarshal(responses[0].Data, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return status
}

func start(t *testing.T, socket, id, root string) protocol.Response {
	t.Helper()

	event := protocol.Event{Event: protocol.SessionStarted, ID: id, Root: root}
	return send(t, socket, protocol.Request{Event: &event})[0]
}

func TestJoiningSessionInheritsTheEstablishedContext(t *testing.T) {
	a := project(t, "music:\n  root: D\n  scale: dorian\n")
	b := project(t, "music:\n  root: A\n  scale: minor_pentatonic\n")

	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if response := start(t, socket, "a1", a); !response.OK {
		t.Fatalf("session.started in project A = %+v", response)
	}
	status := statusOf(t, socket)
	if status.Root != "D2" || status.Scale != "dorian" {
		t.Fatalf("context after project A = root %q scale %q, want D2 dorian", status.Root, status.Scale)
	}
	if status.ContextOwner != a {
		t.Errorf("context owner = %q, want project A at %q", status.ContextOwner, a)
	}

	if response := start(t, socket, "b1", b); !response.OK {
		t.Fatalf("session.started in project B = %+v", response)
	}
	status = statusOf(t, socket)
	if status.Root != "D2" || status.Scale != "dorian" {
		t.Errorf("context after project B joined = root %q scale %q, want project A's D2 dorian retained", status.Root, status.Scale)
	}
	if status.ContextOwner != a {
		t.Errorf("context owner = %q, want project A still attributed", status.ContextOwner)
	}
	if status.SoundingVoices != 2 {
		t.Errorf("sounding voices = %d, want 2 in one shared context", status.SoundingVoices)
	}

	send(t, socket,
		event(protocol.SessionCompleted, "a1"),
		event(protocol.SessionCompleted, "b1"),
	)

	if response := start(t, socket, "b2", b); !response.OK {
		t.Fatalf("session.started in project B once idle = %+v", response)
	}
	status = statusOf(t, socket)
	if status.Root != "A2" || status.Scale != "minor_pentatonic" {
		t.Errorf("context once idle = root %q scale %q, want project B's A2 minor_pentatonic", status.Root, status.Scale)
	}
	if status.ContextOwner != b {
		t.Errorf("context owner = %q, want project B at %q", status.ContextOwner, b)
	}
}

func TestProjectConfigIsHonouredFromAnUnrelatedWorkingDirectory(t *testing.T) {
	proj := project(t, "music:\n  root: F\n  scale: lydian\n")

	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	elsewhere := t.TempDir()
	t.Chdir(elsewhere)

	if response := start(t, socket, "s1", proj); !response.OK {
		t.Fatalf("session.started = %+v", response)
	}

	status := statusOf(t, socket)
	if status.Root != "F2" || status.Scale != "lydian" {
		t.Errorf("context = root %q scale %q, want the project's F2 lydian even though the daemon runs elsewhere", status.Root, status.Scale)
	}
}

func TestSessionWithoutARootUsesGlobalConfig(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if response := start(t, socket, "s1", ""); !response.OK {
		t.Fatalf("session.started without a root = %+v, want ok", response)
	}

	status := statusOf(t, socket)
	if status.Root != "D2" || status.Scale != "minor_pentatonic" {
		t.Errorf("context = root %q scale %q, want the defaults", status.Root, status.Scale)
	}
	if status.ContextOwner != "" {
		t.Errorf("context owner = %q, want it unattributed", status.ContextOwner)
	}
}

func TestSessionWithAMissingRootIsRejected(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	absent := filepath.Join(t.TempDir(), "gone")
	response := start(t, socket, "s1", absent)
	if response.OK {
		t.Fatalf("session.started with a missing root = %+v, want a rejection", response)
	}
	if !strings.Contains(response.Error, "project root") {
		t.Errorf("error %q does not identify the project root as the problem", response.Error)
	}

	if status := statusOf(t, socket); len(status.Sessions) != 0 {
		t.Errorf("status reported %d sessions, want none tracked after a rejected root", len(status.Sessions))
	}
}

func TestSymlinkedRootResolvesToTheCanonicalContext(t *testing.T) {
	proj := project(t, "music:\n  root: G\n  scale: aeolian\n")

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(proj, link); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}

	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if response := start(t, socket, "s1", link); !response.OK {
		t.Fatalf("session.started through a symlink = %+v", response)
	}

	status := statusOf(t, socket)
	if status.Root != "G2" {
		t.Errorf("context root = %q, want G2 resolved through the symlink", status.Root)
	}
	if status.ContextOwner != proj {
		t.Errorf("context owner = %q, want the canonical path %q so a session is not double-counted", status.ContextOwner, proj)
	}
}

func writeUserTheme(t *testing.T, name string, release float64) {
	t.Helper()

	dir := filepath.Join(paths.GlobalConfigDir(), "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`name: %s
waveform: sine
drone:
  attack: 1.0
  release: %v
  gain: 0.5
  harmonic: 0.1
  tremolo_hz: 4.0
  detune_cents: 5.0
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.0
  failure_gain: 0.3
  cancelled_sounds: false
  cancelled_duration: 0.3
  cancelled_gain: 0.3
  attack: 0.02
  decay: 0.1
`, name, release)
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchingThemeMovesTheFadeDeadline(t *testing.T) {
	d, _ := testDaemon(t)
	writeUserTheme(t, "slow", 12.0)

	before := d.releaseWait
	if err := d.useTheme("slow"); err != nil {
		t.Fatalf("useTheme(slow): %v", err)
	}

	want := 12*time.Second + shutdownMargin
	if d.releaseWait != want {
		t.Errorf("releaseWait = %v after switching theme, want %v; shutdown would close audio %v before the new fade finished", d.releaseWait, want, want-d.releaseWait)
	}
	if d.releaseWait == before {
		t.Error("releaseWait did not move with the theme")
	}
}

func TestAdoptingAProjectThemeMovesTheFadeDeadline(t *testing.T) {
	d, _ := testDaemon(t)
	writeUserTheme(t, "sluggish", 9.0)
	proj := project(t, "music:\n  theme: sluggish\n")

	if err := d.adoptContext(proj); err != nil {
		t.Fatalf("adoptContext(%q): %v", proj, err)
	}

	if d.theme.Name != "sluggish" {
		t.Fatalf("theme = %q, want sluggish adopted from the project", d.theme.Name)
	}
	want := 9*time.Second + shutdownMargin
	if d.releaseWait != want {
		t.Errorf("releaseWait = %v, want %v", d.releaseWait, want)
	}
}
