package audio

import (
	"math"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

func testPhraseSpec() theme.PhrasesSpec {
	return theme.PhrasesSpec{
		Attack: 0.005,
		Decay:  0.05,
	}
}

func TestPhraseSource_SilenceBeforeOffset(t *testing.T) {
	f := DefaultFormat()
	offsetDur := 250 * time.Millisecond
	offsetSamples := int(offsetDur.Seconds() * float64(f.SampleRate))

	note := harmony.Note{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Offset:   offsetDur,
		Duration: 50 * time.Millisecond,
		Gain:     0.5,
	}
	src := newPhraseSource(f, note, testPhraseSpec())

	before := make([][2]float32, offsetSamples-1)
	src.Mix(before)
	for i, s := range before {
		if s[0] != 0 || s[1] != 0 {
			t.Fatalf("sample %d has energy before offset elapses", i)
		}
	}

	after := make([][2]float32, 1000)
	src.Mix(after)
	hasEnergy := false
	for _, s := range after {
		if math.Abs(float64(s[0])) > 1e-6 || math.Abs(float64(s[1])) > 1e-6 {
			hasEnergy = true
			break
		}
	}
	if !hasEnergy {
		t.Fatal("expected energy after offset elapses")
	}
}

func TestPhraseSource_SelfRemove(t *testing.T) {
	r := newTestRenderer(t)

	phrase := harmony.Phrase{
		Notes: []harmony.Note{
			{
				Pitch:    harmony.Pitch{Class: 9, Octave: 4},
				Offset:   0,
				Duration: 5 * time.Millisecond,
				Gain:     0.5,
			},
		},
	}
	r.Trigger(phrase)

	if r.mixer.Len() != 1 {
		t.Fatalf("want 1 source after Trigger, got %d", r.mixer.Len())
	}

	buf := make([]byte, DefaultFormat().SampleRate*8)
	for range 100 {
		r.mixer.Read(buf)
		if r.mixer.Len() == 0 {
			return
		}
	}
	t.Fatal("phrase source never self-removed from mixer")
}

func TestPhraseSource_MultiNote(t *testing.T) {
	r := newTestRenderer(t)

	phrase := harmony.Phrase{
		Notes: []harmony.Note{
			{
				Pitch:    harmony.Pitch{Class: 9, Octave: 4},
				Offset:   0,
				Duration: 50 * time.Millisecond,
				Gain:     0.5,
			},
			{
				Pitch:    harmony.Pitch{Class: 0, Octave: 5},
				Offset:   250 * time.Millisecond,
				Duration: 50 * time.Millisecond,
				Gain:     0.5,
			},
		},
	}
	r.Trigger(phrase)

	if r.mixer.Len() != 2 {
		t.Fatalf("want 2 sources for 2-note phrase, got %d", r.mixer.Len())
	}
}

func TestPhraseSource_CapEnforced(t *testing.T) {
	r := newTestRenderer(t)

	phrase := harmony.Phrase{
		Notes: []harmony.Note{
			{
				Pitch:    harmony.Pitch{Class: 9, Octave: 4},
				Offset:   0,
				Duration: 10 * time.Second,
				Gain:     0.5,
			},
		},
	}
	for range 100 {
		r.Trigger(phrase)
	}

	if got := r.mixer.Len(); got > maxPhraseVoices {
		t.Fatalf("phrase voices %d exceed cap %d after 100 Triggers", got, maxPhraseVoices)
	}
}

func TestPhraseSource_ZeroDuration(t *testing.T) {
	f := DefaultFormat()
	note := harmony.Note{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Offset:   0,
		Duration: 0,
		Gain:     0.5,
	}
	src := newPhraseSource(f, note, testPhraseSpec())
	buf := make([][2]float32, 10)
	src.Mix(buf)
	if !src.released {
		t.Fatal("zero-duration note must be released on first Mix")
	}
}

func TestNewPhraseSource_FallbackEnvelope(t *testing.T) {
	f := DefaultFormat()
	note := harmony.Note{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Duration: 50 * time.Millisecond,
		Gain:     0.5,
	}
	src := newPhraseSource(f, note, theme.PhrasesSpec{})
	if src == nil {
		t.Fatal("newPhraseSource must not return nil")
	}
}
