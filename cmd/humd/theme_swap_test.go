package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestThemeSwapDoesNotRetriggerVoices(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if resps := send(t, socket, event(protocol.SessionStarted, "swap-s1")); !resps[0].OK {
		t.Fatalf("session.started = %+v", resps[0])
	}

	historyBefore := len(rec.history())
	initialStatus := statusOf(t, socket)

	themeSwapDir := filepath.Join(os.Getenv("HUM_HOME"), "themes")
	if err := os.MkdirAll(themeSwapDir, 0o755); err != nil {
		t.Fatalf("mkdir themes: %v", err)
	}
	themeSwapYAML := `name: themeswap
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 0.4
  harmonic: 0.15
  tremolo_hz: 5.0
  detune_cents: 8.0
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  cancelled_duration: 0.4
  cancelled_gain: 0.3
  attack: 0.02
  decay: 0.15
`
	const themeSwapName = "themeswap"
	if err := os.WriteFile(filepath.Join(themeSwapDir, themeSwapName+".yaml"), []byte(themeSwapYAML), 0o644); err != nil {
		t.Fatalf("write theme: %v", err)
	}

	resps := send(t, socket, protocol.Request{Command: protocol.CmdThemeUse, Value: themeSwapName})
	if !resps[0].OK {
		t.Fatalf("theme.use %q = %+v, want ok", themeSwapName, resps[0])
	}

	afterStatus := statusOf(t, socket)
	if len(afterStatus.Sessions) != len(initialStatus.Sessions) {
		t.Errorf("sessions = %d after swap, want %d", len(afterStatus.Sessions), len(initialStatus.Sessions))
	}
	if afterStatus.SoundingVoices != initialStatus.SoundingVoices {
		t.Errorf("sounding_voices = %d after swap, want %d", afterStatus.SoundingVoices, initialStatus.SoundingVoices)
	}

	newHistory := rec.history()[historyBefore:]
	sawTheme := false
	for _, call := range newHistory {
		if call == "theme/"+themeSwapName {
			sawTheme = true
		}
		if strings.HasPrefix(call, "trigger/") {
			t.Errorf("recorder saw %q after theme swap, want no trigger calls", call)
		}
	}
	if !sawTheme {
		t.Errorf("recorder history after swap = %v, want \"theme/%s\"", newHistory, themeSwapName)
	}
}
