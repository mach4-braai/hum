//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

var defaultChord = []string{"D3", "F4", "D5", "A4"}

func TestFourConcurrentSessionsSoundTheDocumentedChord(t *testing.T) {
	d := start(t)

	for _, id := range []string{"one", "two", "three", "four"} {
		d.mustHum(t, "start", "--id", id, "--title", id)
	}

	st := d.status(t)
	if st.SoundingVoices != 4 {
		t.Fatalf("sounding voices = %d, want 4", st.SoundingVoices)
	}
	if st.Root != "D3" {
		t.Errorf("root = %q, want D3: the default register is music.octave 3", st.Root)
	}

	got := pitches(st)
	assigned := []string{got["one"], got["two"], got["three"], got["four"]}
	for i, want := range defaultChord {
		if assigned[i] != want {
			t.Errorf("session %d sounds %q, want %q: voices are allocated by interval function, every harmony lifted an octave", i+1, assigned[i], want)
		}
	}

	seen := map[string]bool{}
	for _, p := range assigned {
		if seen[p] {
			t.Errorf("pitch %q allocated twice; four sessions must sound four notes: %v", p, assigned)
		}
		seen[p] = true
	}
}

func TestAReleasedDegreeIsReusedImmediately(t *testing.T) {
	d := start(t)

	for _, id := range []string{"one", "two", "three"} {
		d.mustHum(t, "start", "--id", id, "--title", id)
	}
	freed := pitches(d.status(t))["two"]

	d.mustHum(t, "complete", "--id", "two")
	d.mustHum(t, "start", "--id", "four", "--title", "four")

	if got := pitches(d.status(t))["four"]; got != freed {
		t.Errorf("the new session sounds %q, want the freed %q: lowest-free allocation makes a released degree reusable without re-voicing the drones still sounding", got, freed)
	}
}

func TestUpdatesChangeExpressionNotPitch(t *testing.T) {
	d := start(t)
	d.mustHum(t, "start", "--id", "busy", "--title", "busy")

	pitch := d.status(t).Sessions[0].Pitch

	for range 20 {
		d.mustHum(t, "update", "--id", "busy")
	}

	after := d.status(t)
	if len(after.Sessions) != 1 {
		t.Fatalf("session count = %d after twenty updates, want 1", len(after.Sessions))
	}
	if after.Sessions[0].Pitch != pitch {
		t.Errorf("pitch = %q after twenty updates, want %q unchanged: an update routes to expression, never to a new note", after.Sessions[0].Pitch, pitch)
	}
	if after.Sessions[0].Updates != 20 {
		t.Errorf("updates = %d, want 20: the burst must be recorded even though it is silent in the log", after.Sessions[0].Updates)
	}
	if after.SoundingVoices != 1 {
		t.Errorf("sounding voices = %d, want 1", after.SoundingVoices)
	}
}

func TestCompletionAndFailureEmptyTheSoundscape(t *testing.T) {
	d := start(t)
	d.mustHum(t, "start", "--id", "good", "--title", "good")
	d.mustHum(t, "start", "--id", "bad", "--title", "bad")

	d.mustHum(t, "complete", "--id", "good")
	d.mustHum(t, "fail", "--id", "bad")

	st := d.status(t)
	if st.SoundingVoices != 0 {
		t.Errorf("sounding voices = %d after both sessions ended, want 0", st.SoundingVoices)
	}
	states := map[string]string{}
	for _, s := range st.Sessions {
		states[s.ID] = s.State
	}
	if states["good"] != "completed" || states["bad"] != "failed" {
		t.Errorf("terminal states = %v, want good completed and bad failed", states)
	}
}

func TestCancellingASessionIsSilentButTracked(t *testing.T) {
	d := start(t)
	d.mustHum(t, "start", "--id", "gone", "--title", "gone")
	d.mustHum(t, "cancel", "--id", "gone")

	st := d.status(t)
	if st.SoundingVoices != 0 {
		t.Errorf("sounding voices = %d after a cancel, want 0", st.SoundingVoices)
	}
	if st.Sessions[0].State != "cancelled" {
		t.Errorf("state = %q, want cancelled", st.Sessions[0].State)
	}
}

func TestShutdownFadesEveryVoiceBeforeClosingTheDevice(t *testing.T) {
	d := start(t)
	for _, id := range []string{"one", "two", "three", "four"} {
		d.mustHum(t, "start", "--id", id, "--title", id)
	}
	if d.status(t).SoundingVoices != 4 {
		t.Fatal("four voices must be sounding for the shutdown to mean anything")
	}

	out, code := d.hum(t, "stop")
	if code != 0 {
		t.Fatalf("hum stop exited %d: %s", code, out)
	}

	d.waitForLog(t, "waiting for voices to fade", 15*time.Second)
	d.waitForLog(t, "stopped", 15*time.Second)

	if got := d.waitExit(t, 15*time.Second); got != 0 {
		t.Errorf("humd exited %d after a clean stop, want 0", got)
	}
	if _, err := os.Stat(d.socket); err == nil {
		t.Error("the socket outlived the daemon")
	}
}

func TestDoctorReportsTheSoundingRegister(t *testing.T) {
	d := start(t)
	out := d.mustHum(t, "doctor")

	if !strings.Contains(out, "root D3, scale minor_pentatonic") {
		t.Errorf("doctor does not name the sounding pitch:\n%s", out)
	}
	if !strings.Contains(out, "music.octave") {
		t.Errorf("doctor omits the register from its provenance table:\n%s", out)
	}
}
