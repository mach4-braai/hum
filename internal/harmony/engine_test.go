package harmony

import (
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/session"
)

func makeEngine(t *testing.T) (*Engine, Pitch, Scale) {
	t.Helper()
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	return NewEngine(root, scale, DefaultPhraseSpec()), root, scale
}

func addChange(id string) session.Change {
	return session.Change{
		Kind:    session.ChangeAdded,
		Session: session.Session{ID: id, State: session.StateActive},
	}
}

func updateChange(id string, metadata map[string]string) session.Change {
	return session.Change{
		Kind:    session.ChangeUpdated,
		Session: session.Session{ID: id, State: session.StateActive, Metadata: metadata},
	}
}

func endChange(id string, state session.State) session.Change {
	return session.Change{
		Kind:    session.ChangeEnded,
		Session: session.Session{ID: id, State: state},
	}
}

func TestEngineAddAcquiresVoice(t *testing.T) {
	eng, _, _ := makeEngine(t)

	st, phrases := eng.Apply(addChange("s0"))

	if len(phrases) != 0 {
		t.Errorf("ChangeAdded: want no phrases, got %d", len(phrases))
	}
	if len(st.Voices) != 1 {
		t.Fatalf("want 1 voice, got %d", len(st.Voices))
	}
	if st.Voices[0].Degree != 0 {
		t.Errorf("first voice: want degree 0, got %d", st.Voices[0].Degree)
	}
}

func TestEngineCompletionD2(t *testing.T) {
	eng, _, _ := makeEngine(t)

	eng.Apply(addChange("s0"))
	st, phrases := eng.Apply(endChange("s0", session.StateCompleted))

	if len(st.Voices) != 0 {
		t.Errorf("after completion: want empty State.Voices, got %d", len(st.Voices))
	}
	if len(phrases) != 1 {
		t.Fatalf("want 1 phrase, got %d", len(phrases))
	}
	p := phrases[0]
	if p.Kind != PhraseCompletion {
		t.Errorf("want PhraseCompletion, got %q", p.Kind)
	}
	if len(p.Notes) != 1 {
		t.Fatalf("completion: want 1 note, got %d", len(p.Notes))
	}
	want, _ := ParsePitch("D4")
	if p.Notes[0].Pitch != want {
		t.Errorf("completion pitch: want D4 (%v), got %v", want, p.Notes[0].Pitch)
	}
	if p.Notes[0].Duration != DefaultPhraseSpec().CompletionDuration {
		t.Errorf("completion duration mismatch")
	}
	if p.Notes[0].Gain != DefaultPhraseSpec().CompletionGain {
		t.Errorf("completion gain mismatch")
	}
}

func TestEngineFailureDescends(t *testing.T) {
	eng, _, _ := makeEngine(t)

	eng.Apply(addChange("s0"))
	st, phrases := eng.Apply(endChange("s0", session.StateFailed))

	if len(st.Voices) != 0 {
		t.Errorf("after failure: want empty State.Voices, got %d", len(st.Voices))
	}
	if len(phrases) != 1 {
		t.Fatalf("want 1 phrase, got %d", len(phrases))
	}
	p := phrases[0]
	if p.Kind != PhraseFailure {
		t.Errorf("want PhraseFailure, got %q", p.Kind)
	}
	if len(p.Notes) != 2 {
		t.Fatalf("failure: want 2 notes, got %d", len(p.Notes))
	}
	if p.Notes[1].Pitch.Midi() >= p.Notes[0].Pitch.Midi() {
		t.Errorf("failure: second note must be below first: %v >= %v",
			p.Notes[1].Pitch, p.Notes[0].Pitch)
	}
	if p.Notes[1].Offset != DefaultPhraseSpec().FailureDuration {
		t.Errorf("failure: second note offset should equal FailureDuration")
	}
}

func TestEngineCancelledSilentByDefault(t *testing.T) {
	eng, _, _ := makeEngine(t)

	eng.Apply(addChange("s0"))
	st, phrases := eng.Apply(endChange("s0", session.StateCancelled))

	if len(st.Voices) != 0 {
		t.Errorf("after cancelled: want empty State.Voices, got %d", len(st.Voices))
	}
	if len(phrases) != 0 {
		t.Errorf("cancelled (default): want no phrases, got %d", len(phrases))
	}
}

func TestEngineCancelledWithSounds(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	spec := DefaultPhraseSpec()
	spec.CancelledSounds = true
	eng := NewEngine(root, scale, spec)

	eng.Apply(addChange("s0"))
	_, phrases := eng.Apply(endChange("s0", session.StateCancelled))

	if len(phrases) != 1 {
		t.Fatalf("CancelledSounds=true: want 1 phrase, got %d", len(phrases))
	}
	if phrases[0].Kind != PhraseCancelled {
		t.Errorf("want PhraseCancelled, got %q", phrases[0].Kind)
	}
	note := phrases[0].Notes[0]
	if note.Duration != spec.CancelledDuration {
		t.Errorf("cancelled note duration = %v, want the spec's %v", note.Duration, spec.CancelledDuration)
	}
	if note.Gain != spec.CancelledGain {
		t.Errorf("cancelled note gain = %v, want the spec's %v", note.Gain, spec.CancelledGain)
	}
	if note.Pitch != mustParsePitch(t, "D2") {
		t.Errorf("cancelled note pitch = %v, want the sounding pitch D2", note.Pitch)
	}
}

func TestEngineStateOrderDeterministic(t *testing.T) {
	eng, _, _ := makeEngine(t)

	eng.Apply(addChange("s2"))
	eng.Apply(addChange("s0"))
	eng.Apply(addChange("s1"))

	st, _ := eng.Apply(updateChange("s0", nil))

	if len(st.Voices) != 3 {
		t.Fatalf("want 3 voices, got %d", len(st.Voices))
	}
	for i := 1; i < len(st.Voices); i++ {
		if st.Voices[i].Degree < st.Voices[i-1].Degree {
			t.Errorf("state not sorted at index %d: degree %d < %d",
				i, st.Voices[i].Degree, st.Voices[i-1].Degree)
		}
	}
	if st.Voices[0].Degree != 0 || st.Voices[1].Degree != 1 || st.Voices[2].Degree != 5 {
		t.Errorf("degrees = %v, want the root, its third and its octave", st.Voices)
	}
}

func TestEngineUpdatePreservesPitchAndDegree(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stageNow(t, base)

	eng, _, _ := makeEngine(t)
	eng.Apply(addChange("s0"))

	st0, _ := eng.Apply(updateChange("s0", nil))
	pitch0 := st0.Voices[0].Pitch
	degree0 := st0.Voices[0].Degree

	for range 10 {
		eng.Apply(updateChange("s0", nil))
	}
	st1, _ := eng.Apply(updateChange("s0", nil))

	if st1.Voices[0].Pitch != pitch0 {
		t.Errorf("updates changed pitch: %v → %v", pitch0, st1.Voices[0].Pitch)
	}
	if st1.Voices[0].Degree != degree0 {
		t.Errorf("updates changed degree: %d → %d", degree0, st1.Voices[0].Degree)
	}
}

func TestEngineUpdateRaisesIntensity(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := stageNow(t, base)

	eng, _, _ := makeEngine(t)
	eng.Apply(addChange("s0"))

	st0, _ := eng.Apply(updateChange("s0", nil))
	initial := st0.Voices[0].Expression.Intensity

	for range 10 {
		eng.Apply(updateChange("s0", nil))
	}
	st1, _ := eng.Apply(updateChange("s0", nil))
	raised := st1.Voices[0].Expression.Intensity

	if raised <= initial {
		t.Errorf("10 updates: want Intensity > %v, got %v", initial, raised)
	}

	*ts = base.Add(100 * time.Second)
	st2, _ := eng.Apply(updateChange("s0", nil))
	decayed := st2.Voices[0].Expression.Intensity

	if decayed >= raised {
		t.Errorf("after decay: want Intensity < %v, got %v", raised, decayed)
	}
}

func TestEngineUpdateUnknownSessionSafe(t *testing.T) {
	eng, _, _ := makeEngine(t)

	st, phrases := eng.Apply(updateChange("ghost", nil))

	if len(phrases) != 0 {
		t.Errorf("update unknown: want no phrases, got %d", len(phrases))
	}
	if len(st.Voices) != 0 {
		t.Errorf("update unknown: want empty state, got %d voices", len(st.Voices))
	}
}

func TestEngineEndUnknownSessionNoPanic(t *testing.T) {
	eng, _, _ := makeEngine(t)

	st, phrases := eng.Apply(endChange("nobody", session.StateCompleted))

	if len(phrases) != 0 {
		t.Errorf("end unknown: want no phrases, got %d", len(phrases))
	}
	if len(st.Voices) != 0 {
		t.Errorf("end unknown: want empty state, got %d voices", len(st.Voices))
	}
}

func TestEngineUnknownChangeKind(t *testing.T) {
	eng, _, _ := makeEngine(t)

	st, phrases := eng.Apply(session.Change{Kind: session.ChangeKind("bogus")})

	if len(phrases) != 0 {
		t.Errorf("bogus kind: want no phrases, got %d", len(phrases))
	}
	if len(st.Voices) != 0 {
		t.Errorf("bogus kind: want empty state, got %d voices", len(st.Voices))
	}
}

func TestEngineAgentsWidthViaMetadata(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stageNow(t, base)

	eng, _, _ := makeEngine(t)
	eng.Apply(addChange("s0"))

	st1, _ := eng.Apply(updateChange("s0", map[string]string{"agents": "1"}))
	st10, _ := eng.Apply(updateChange("s0", map[string]string{"agents": "10"}))

	w1 := st1.Voices[0].Expression.Width
	w10 := st10.Voices[0].Expression.Width

	if w10 <= w1 {
		t.Errorf("agents=10 should give higher Width than agents=1: %v <= %v", w10, w1)
	}
}

func TestEngineRetune(t *testing.T) {
	eng, _, _ := makeEngine(t)

	root2 := mustParsePitch(t, "A3")
	scale2 := mustLookupScale(t, "major")

	if err := eng.Retune(root2, scale2); err != nil {
		t.Fatalf("Retune when idle: unexpected error: %v", err)
	}

	p, s := eng.Tuning()
	if p != root2 {
		t.Errorf("Tuning root: want %v, got %v", root2, p)
	}
	if s.Name != scale2.Name {
		t.Errorf("Tuning scale: want %v, got %v", scale2.Name, s.Name)
	}

	eng.Apply(addChange("s0"))

	if err := eng.Retune(root2, scale2); err == nil {
		t.Error("Retune while sounding: want error")
	} else if err != ErrRetuneBusy {
		t.Errorf("Retune while sounding: want ErrRetuneBusy, got %v", err)
	}
}

func TestEngineTuning(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	eng := NewEngine(root, scale, DefaultPhraseSpec())

	p, s := eng.Tuning()
	if p != root {
		t.Errorf("Tuning: want root %v, got %v", root, p)
	}
	if s.Name != scale.Name {
		t.Errorf("Tuning: want scale %v, got %v", scale.Name, s.Name)
	}
}

func TestEnginePhraseEmittedBeforeRelease(t *testing.T) {
	eng, _, _ := makeEngine(t)

	eng.Apply(addChange("s0"))
	_, phrases := eng.Apply(endChange("s0", session.StateCompleted))

	if len(phrases) == 0 {
		t.Fatal("want phrase, got none")
	}
	d2, _ := ParsePitch("D2")
	voicePitch := eng.alloc.scale.Degree(eng.alloc.root, 0)
	if voicePitch != d2 {
		t.Errorf("root voice pitch: want D2, got %v", voicePitch)
	}

	d4, _ := ParsePitch("D4")
	if phrases[0].Notes[0].Pitch != d4 {
		t.Errorf("completion note: want D4, got %v", phrases[0].Notes[0].Pitch)
	}
}

func TestEngineDefaultPhraseSpec(t *testing.T) {
	spec := DefaultPhraseSpec()
	if spec.CompletionOctaves != 2 {
		t.Errorf("CompletionOctaves: want 2, got %d", spec.CompletionOctaves)
	}
	if spec.FailureInterval != -3 {
		t.Errorf("FailureInterval: want -3, got %d", spec.FailureInterval)
	}
	if spec.CancelledSounds {
		t.Error("CancelledSounds: want false")
	}
	if spec.FailureDuration <= spec.CompletionDuration {
		t.Errorf("FailureDuration should exceed CompletionDuration: %v <= %v",
			spec.FailureDuration, spec.CompletionDuration)
	}
	if spec.FailureGain >= spec.CompletionGain {
		t.Errorf("FailureGain should be quieter than CompletionGain: %v >= %v",
			spec.FailureGain, spec.CompletionGain)
	}
}
