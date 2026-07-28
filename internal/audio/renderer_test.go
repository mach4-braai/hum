package audio

import (
	"math"
	"testing"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
)

func testOpts() renderer.Options {
	return renderer.Options{
		SampleRate: 48000,
		Volume:     0.6,
		Theme: theme.Theme{
			Drone: theme.DroneSpec{
				Attack:  0.01,
				Release: 0.05,
				Gain:    0.7,
			},
			Phrases: theme.PhrasesSpec{
				Attack: 0.005,
				Decay:  0.05,
			},
		},
	}
}

func newTestRenderer(t *testing.T) *AudioRenderer {
	t.Helper()
	f := DefaultFormat()
	m := NewMixer(f)
	r := newRendererWithMixer(m, f, testOpts())
	r.rampDuration = 0
	return r
}

func voiceState(sessionID string, class, octave int) harmony.VoiceState {
	return harmony.VoiceState{
		Voice: harmony.Voice{
			SessionID: sessionID,
			Pitch:     harmony.Pitch{Class: class, Octave: octave},
		},
	}
}

func TestUpdate_AddVoiceCreatesOneOsc(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state); err != nil {
		t.Fatal(err)
	}
	if got := r.mixer.Len(); got != 1 {
		t.Fatalf("want 1 osc, got %d", got)
	}
}

func TestUpdate_ReleaseNotDelete(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	r.Update(state)

	r.Update(harmony.State{})

	if got := r.mixer.Len(); got != 1 {
		t.Fatalf("released voice should still be in mixer, got Len=%d", got)
	}

	buf := make([]byte, DefaultFormat().SampleRate*8)
	for range 100 {
		r.mixer.Read(buf)
		if r.mixer.Len() == 0 {
			return
		}
	}
	t.Fatal("released voice never removed from mixer after draining samples")
}

func TestUpdate_Idempotent(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}

	r.Update(state)
	if r.mixer.Len() != 1 {
		t.Fatal("want 1 after first Update")
	}

	r.Update(state)
	if r.mixer.Len() != 1 {
		t.Fatal("idempotent Update must not create a second osc")
	}
}

func TestUpdate_Idempotent_NoGainReset(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	r.Update(state)

	buf := make([]byte, 512)
	r.mixer.Read(buf)

	gainBefore := r.mixer.Gain()
	r.Update(state)
	gainAfter := r.mixer.Gain()

	if gainBefore != gainAfter {
		t.Fatalf("idempotent Update must not change mixer gain: before=%v after=%v", gainBefore, gainAfter)
	}
}

func TestSetVolume_NaN(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.SetVolume(math.NaN()); err == nil {
		t.Fatal("SetVolume(NaN) must return an error")
	}
}

func TestSetVolume_OutOfRange(t *testing.T) {
	r := newTestRenderer(t)
	for _, bad := range []float64{-0.1, 1.1, math.Inf(1), math.Inf(-1)} {
		if err := r.SetVolume(bad); err == nil {
			t.Fatalf("SetVolume(%v) must return an error", bad)
		}
	}
}

func TestSetVolume_Valid(t *testing.T) {
	r := newTestRenderer(t)
	for _, v := range []float64{0, 0.5, 1} {
		if err := r.SetVolume(v); err != nil {
			t.Fatalf("SetVolume(%v) unexpected error: %v", v, err)
		}
	}
}

func TestSetMuted_RestoresVolume(t *testing.T) {
	r := newTestRenderer(t)
	r.SetVolume(0.6)

	r.SetMuted(true)
	r.SetMuted(false)

	if got := r.mixer.Gain(); got != 0.6 {
		t.Fatalf("want gain 0.6 after unmute, got %v", got)
	}
}

func TestSetMuted_Idempotent(t *testing.T) {
	r := newTestRenderer(t)
	r.SetMuted(true)
	r.SetMuted(true)
	r.SetMuted(false)
	r.SetMuted(false)
}

func TestClose_Safe(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClose_UpdateAfterClose(t *testing.T) {
	r := newTestRenderer(t)
	r.Close()
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state); err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 0 {
		t.Fatal("Update after Close must not add sources")
	}
}

func TestName(t *testing.T) {
	r := newTestRenderer(t)
	if r.Name() != "audio" {
		t.Fatalf("want name audio, got %q", r.Name())
	}
}

func TestTrigger_PhraseCap(t *testing.T) {
	r := newTestRenderer(t)
	phrase := harmony.Phrase{
		Notes: []harmony.Note{
			{
				Pitch:    harmony.Pitch{Class: 9, Octave: 4},
				Offset:   0,
				Duration: 100e6,
				Gain:     0.5,
			},
		},
	}
	for range 100 {
		r.Trigger(phrase)
	}
	if got := r.mixer.Len(); got > maxPhraseVoices {
		t.Fatalf("phrase voices %d exceed cap %d", got, maxPhraseVoices)
	}
}

func TestTrigger_AfterClose(t *testing.T) {
	r := newTestRenderer(t)
	r.Close()
	err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Duration: 100e6,
		Gain:     0.5,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 0 {
		t.Fatal("Trigger after Close must not add sources")
	}
}

func TestRegistered(t *testing.T) {
	names := renderer.Names()
	found := false
	for _, n := range names {
		if n == "audio" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("audio renderer not registered")
	}
}
