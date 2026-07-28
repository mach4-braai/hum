package renderer

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

func saveRegistry(t *testing.T) {
	t.Helper()
	regMu.Lock()
	saved := make(map[string]constructor, len(registry))
	for k, v := range registry {
		saved[k] = v
	}
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		registry = saved
		regMu.Unlock()
	})
}

func TestRegisterDuplicatePanics(t *testing.T) {
	saveRegistry(t)
	Register("dup_test", func(Options) (Renderer, error) { return nil, nil })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
	}()
	Register("dup_test", func(Options) (Renderer, error) { return nil, nil })
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	saveRegistry(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty name")
		}
	}()
	Register("", func(Options) (Renderer, error) { return nil, nil })
}

func TestRegisterNilCtorPanics(t *testing.T) {
	saveRegistry(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil constructor")
		}
	}()
	Register("nil_ctor_test", nil)
}

func TestOpenUnknownNameErrors(t *testing.T) {
	saveRegistry(t)
	Register("known_a", func(Options) (Renderer, error) { return nil, nil })
	Register("known_b", func(Options) (Renderer, error) { return nil, nil })

	_, err := Open("not_there", Options{})
	if err == nil {
		t.Fatal("expected error for unknown renderer")
	}
	msg := err.Error()
	if !strings.Contains(msg, "known_a") || !strings.Contains(msg, "known_b") {
		t.Fatalf("error message should list known names, got: %s", msg)
	}
}

func TestOpenNopDefaults(t *testing.T) {
	r, err := Open("nop", Options{})
	if err != nil {
		t.Fatalf("Open(nop) error: %v", err)
	}
	nop := r.(*NopRenderer)
	if nop.opts.SampleRate != defaultSampleRate {
		t.Errorf("SampleRate: got %d, want %d", nop.opts.SampleRate, defaultSampleRate)
	}
	if nop.opts.Logger == nil {
		t.Error("Logger should be non-nil after defaulting")
	}
	if nop.Volume() == 0 {
		t.Error("Volume should be non-zero after defaulting")
	}
}

func TestOpenNopWithThemeVolume(t *testing.T) {
	th := theme.Theme{Name: "test_theme", Drone: theme.DroneSpec{Gain: 0.75}}
	r, err := Open("nop", Options{Theme: th})
	if err != nil {
		t.Fatalf("Open(nop) error: %v", err)
	}
	nop := r.(*NopRenderer)
	if nop.Volume() != 0.75 {
		t.Errorf("Volume: got %v, want 0.75", nop.Volume())
	}
}

func TestNamesIsSorted(t *testing.T) {
	saveRegistry(t)
	Register("zz_test", func(Options) (Renderer, error) { return nil, nil })
	Register("aa_test", func(Options) (Renderer, error) { return nil, nil })

	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted at index %d: %q before %q", i, names[i-1], names[i])
		}
	}
}

func TestNamesMutationDoesNotAffectRegistry(t *testing.T) {
	saveRegistry(t)
	Register("mut_test", func(Options) (Renderer, error) { return nil, nil })

	n1 := Names()
	before := len(n1)
	n1[0] = "mutated_value"

	n2 := Names()
	if len(n2) != before {
		t.Errorf("registry length changed after slice mutation: got %d, want %d", len(n2), before)
	}
	for _, name := range n2 {
		if name == "mutated_value" {
			t.Error("mutation of returned slice corrupted registry")
		}
	}
}

func TestNopRecordsUpdatesInOrder(t *testing.T) {
	n := NewNop(Options{})

	s1 := harmony.State{Voices: []harmony.VoiceState{{Voice: harmony.Voice{SessionID: "a"}}}}
	s2 := harmony.State{Voices: []harmony.VoiceState{{Voice: harmony.Voice{SessionID: "b"}}}}

	if err := n.Update(s1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := n.Update(s2); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := n.Updates()
	if len(got) != 2 {
		t.Fatalf("Updates() length: got %d, want 2", len(got))
	}
	if got[0].Voices[0].Voice.SessionID != "a" {
		t.Errorf("Updates()[0]: got %q, want %q", got[0].Voices[0].Voice.SessionID, "a")
	}
	if got[1].Voices[0].Voice.SessionID != "b" {
		t.Errorf("Updates()[1]: got %q, want %q", got[1].Voices[0].Voice.SessionID, "b")
	}
}

func TestNopUpdatesReturnsCopies(t *testing.T) {
	n := NewNop(Options{})

	s := harmony.State{Voices: []harmony.VoiceState{{Voice: harmony.Voice{SessionID: "original"}}}}
	if err := n.Update(s); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := n.Updates()
	got[0].Voices[0].Voice.SessionID = "mutated"

	got2 := n.Updates()
	if got2[0].Voices[0].Voice.SessionID != "original" {
		t.Errorf("mutation of returned slice corrupted recorder: got %q", got2[0].Voices[0].Voice.SessionID)
	}
}

func TestNopRecordsTriggersInOrder(t *testing.T) {
	n := NewNop(Options{})

	p1 := harmony.Phrase{Kind: harmony.PhraseCompletion, Notes: []harmony.Note{{Gain: 1.0}}}
	p2 := harmony.Phrase{Kind: harmony.PhraseFailure, Notes: []harmony.Note{{Gain: 0.5}}}

	if err := n.Trigger(p1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if err := n.Trigger(p2); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	got := n.Triggers()
	if len(got) != 2 {
		t.Fatalf("Triggers() length: got %d, want 2", len(got))
	}
	if got[0].Kind != harmony.PhraseCompletion {
		t.Errorf("Triggers()[0].Kind: got %q, want completion", got[0].Kind)
	}
	if got[1].Kind != harmony.PhraseFailure {
		t.Errorf("Triggers()[1].Kind: got %q, want failure", got[1].Kind)
	}
}

func TestNopTriggersReturnsCopies(t *testing.T) {
	n := NewNop(Options{})

	p := harmony.Phrase{Kind: harmony.PhraseCompletion, Notes: []harmony.Note{{Offset: time.Second}}}
	if err := n.Trigger(p); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	got := n.Triggers()
	got[0].Notes[0].Offset = 999 * time.Second

	got2 := n.Triggers()
	if got2[0].Notes[0].Offset != time.Second {
		t.Errorf("mutation of returned slice corrupted recorder: got %v", got2[0].Notes[0].Offset)
	}
}

func TestNopSetVolumeNaN(t *testing.T) {
	n := NewNop(Options{})
	if err := n.SetVolume(math.NaN()); err == nil {
		t.Error("SetVolume(NaN) should return error")
	}
}

func TestNopSetVolumeOutOfRange(t *testing.T) {
	n := NewNop(Options{})
	if err := n.SetVolume(-0.1); err == nil {
		t.Error("SetVolume(-0.1) should return error")
	}
	if err := n.SetVolume(1.1); err == nil {
		t.Error("SetVolume(1.1) should return error")
	}
}

func TestNopSetVolumeValid(t *testing.T) {
	n := NewNop(Options{})
	if err := n.SetVolume(0.5); err != nil {
		t.Errorf("SetVolume(0.5) unexpected error: %v", err)
	}
	if n.Volume() != 0.5 {
		t.Errorf("Volume(): got %v, want 0.5", n.Volume())
	}
}

func TestNopSetMuted(t *testing.T) {
	n := NewNop(Options{})
	if err := n.SetMuted(true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	if !n.Muted() {
		t.Error("Muted() should be true after SetMuted(true)")
	}
	if err := n.SetMuted(false); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	if n.Muted() {
		t.Error("Muted() should be false after SetMuted(false)")
	}
}

func TestNopCloseSafeTwice(t *testing.T) {
	n := NewNop(Options{})
	if err := n.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}
	if n.Closes() != 2 {
		t.Errorf("Closes(): got %d, want 2", n.Closes())
	}
}

func TestNopName(t *testing.T) {
	n := NewNop(Options{})
	if n.Name() != "nop" {
		t.Errorf("Name(): got %q, want %q", n.Name(), "nop")
	}
}

func TestNopSatisfiesRenderer(t *testing.T) {
	var _ Renderer = (*NopRenderer)(nil)
}

func TestNopOptionsDefaultSampleRate(t *testing.T) {
	n := NewNop(Options{})
	if n.opts.SampleRate != defaultSampleRate {
		t.Errorf("SampleRate: got %d, want %d", n.opts.SampleRate, defaultSampleRate)
	}
}

func TestNopOptionsPreservesExplicitSampleRate(t *testing.T) {
	n := NewNop(Options{SampleRate: 44100})
	if n.opts.SampleRate != 44100 {
		t.Errorf("SampleRate: got %d, want 44100", n.opts.SampleRate)
	}
}

func TestNopOptionsDefaultVolumeNoTheme(t *testing.T) {
	n := NewNop(Options{})
	if n.Volume() != defaultVolume {
		t.Errorf("Volume(): got %v, want %v", n.Volume(), defaultVolume)
	}
}

func TestApplyDefaultsVolumeFromTheme(t *testing.T) {
	th := theme.Theme{Name: "x", Drone: theme.DroneSpec{Gain: 0.42}}
	opts := applyDefaults(Options{Theme: th})
	if opts.Volume != 0.42 {
		t.Errorf("Volume: got %v, want 0.42", opts.Volume)
	}
}

func TestApplyDefaultsExplicitVolumePreserved(t *testing.T) {
	opts := applyDefaults(Options{Volume: 0.3})
	if opts.Volume != 0.3 {
		t.Errorf("Volume: got %v, want 0.3", opts.Volume)
	}
}

func TestApplyDefaultsLoggerDefault(t *testing.T) {
	opts := applyDefaults(Options{})
	if opts.Logger == nil {
		t.Error("Logger should be non-nil after applyDefaults")
	}
}

func TestOpenErrorListsRegistered(t *testing.T) {
	names := Names()
	_, err := Open("nonexistent_xyz", Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, name := range names {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error message missing registered name %q: %s", name, err.Error())
		}
	}
}
