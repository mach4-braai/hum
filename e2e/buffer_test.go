//go:build e2e

package e2e

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/audio"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/session"
	"github.com/mach4-braai/hum/internal/theme"
)

const captureRate = 48000

var droneRoot = harmony.Pitch{Class: 2, Octave: 3}

type capture struct {
	renderer *audio.AudioRenderer
	mixer    *audio.Mixer
	engine   *harmony.Engine
	registry *session.Registry
	spec     harmony.PhraseSpec
	release  time.Duration
}

func newCapture(t *testing.T) *capture {
	t.Helper()

	th, err := theme.Load("minimal")
	if err != nil {
		t.Fatalf("load minimal theme: %v", err)
	}
	scale, err := harmony.LookupScale("minor_pentatonic")
	if err != nil {
		t.Fatalf("lookup scale: %v", err)
	}

	format := audio.Format{SampleRate: captureRate, Channels: 2}
	r, m := audio.NewCaptureRenderer(format, renderer.Options{
		SampleRate: captureRate,
		Theme:      th,
		Volume:     1,
	})
	t.Cleanup(func() { r.Close() })

	return &capture{
		renderer: r,
		mixer:    m,
		engine:   harmony.NewEngine(droneRoot, scale, th.PhraseSpec()),
		registry: session.New(),
		spec:     th.PhraseSpec(),
		release:  time.Duration(th.Drone.Release * float64(time.Second)),
	}
}

func (c *capture) apply(t *testing.T, event protocol.Event) []harmony.Phrase {
	t.Helper()
	change, err := c.registry.Apply(event)
	if err != nil {
		t.Fatalf("%s %s: %v", event.Event, event.ID, err)
	}
	state, phrases := c.engine.Apply(change)
	if err := c.renderer.Update(state); err != nil {
		t.Fatalf("renderer update: %v", err)
	}
	for _, p := range phrases {
		if err := c.renderer.Trigger(p); err != nil {
			t.Fatalf("renderer trigger: %v", err)
		}
	}
	return phrases
}

func (c *capture) read(t *testing.T, d time.Duration) []float64 {
	t.Helper()
	frames := int(d.Seconds() * captureRate)
	buf := make([]byte, frames*8)
	if _, err := c.mixer.Read(buf); err != nil {
		t.Fatalf("mixer read: %v", err)
	}
	out := make([]float64, frames)
	for i := range out {
		out[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i*8:])))
	}
	return out
}

func goertzel(samples []float64, freq float64) float64 {
	w := 2 * math.Cos(2*math.Pi*freq/captureRate)
	var s1, s2 float64
	for _, x := range samples {
		s := x + w*s1 - s2
		s2 = s1
		s1 = s
	}
	power := s1*s1 + s2*s2 - w*s1*s2
	return math.Sqrt(math.Max(power, 0)) / float64(len(samples))
}

func peak(samples []float64) float64 {
	out := 0.0
	for _, x := range samples {
		if a := math.Abs(x); a > out {
			out = a
		}
	}
	return out
}

func TestTheDroneSoundsItsAllocatedPitch(t *testing.T) {
	c := newCapture(t)
	c.apply(t, protocol.Event{Event: protocol.SessionStarted, ID: "one"})

	c.read(t, 3*time.Second)
	samples := c.read(t, 500*time.Millisecond)

	atPitch := goertzel(samples, droneRoot.Freq())
	aTritoneAway := goertzel(samples, droneRoot.Transpose(6).Freq())

	if atPitch <= 10*aTritoneAway {
		t.Errorf("energy at %s (%.5g) is not dominant over a tritone away (%.5g); the drone is not sounding its allocated pitch", droneRoot, atPitch, aTritoneAway)
	}
}

func TestCompletionSoundsAboveItsDrone(t *testing.T) {
	c := newCapture(t)
	c.apply(t, protocol.Event{Event: protocol.SessionStarted, ID: "one"})
	c.read(t, 3*time.Second)

	phrases := c.apply(t, protocol.Event{Event: protocol.SessionCompleted, ID: "one"})
	if len(phrases) != 1 || phrases[0].Kind != harmony.PhraseCompletion {
		t.Fatalf("phrases = %+v, want one completion", phrases)
	}

	want := droneRoot.Transpose(c.spec.CompletionOctaves * 12)
	if got := phrases[0].Notes[0].Pitch; got != want {
		t.Fatalf("completion note = %s, want %s: %d octaves above the drone", got, want, c.spec.CompletionOctaves)
	}
	if want.Midi() <= droneRoot.Midi() {
		t.Fatalf("completion note %s is not above the drone %s", want, droneRoot)
	}

	samples := c.read(t, c.spec.CompletionDuration)
	atPhrase := goertzel(samples, want.Freq())
	oneOctaveOff := goertzel(samples, want.Transpose(12).Freq())

	if atPhrase <= 5*oneOctaveOff {
		t.Errorf("energy at the completion pitch %s (%.5g) is not dominant over an octave above it (%.5g)", want, atPhrase, oneOctaveOff)
	}
}

func TestFailureDescendsByTheDocumentedInterval(t *testing.T) {
	c := newCapture(t)
	c.apply(t, protocol.Event{Event: protocol.SessionStarted, ID: "one"})
	c.read(t, 3*time.Second)

	phrases := c.apply(t, protocol.Event{Event: protocol.SessionFailed, ID: "one"})
	if len(phrases) != 1 || phrases[0].Kind != harmony.PhraseFailure {
		t.Fatalf("phrases = %+v, want one failure", phrases)
	}
	notes := phrases[0].Notes
	if len(notes) != 2 {
		t.Fatalf("failure carries %d notes, want 2", len(notes))
	}
	if c.spec.FailureInterval >= 0 {
		t.Fatalf("failure interval is %+d semitones; the cadence must descend", c.spec.FailureInterval)
	}
	if got, want := notes[1].Pitch, notes[0].Pitch.Transpose(c.spec.FailureInterval); got != want {
		t.Fatalf("second failure note = %s, want %s", got, want)
	}
	if notes[1].Pitch.Midi() >= notes[0].Pitch.Midi() {
		t.Fatalf("second failure note %s is not below the first %s", notes[1].Pitch, notes[0].Pitch)
	}

	first := c.read(t, c.spec.FailureDuration/2)
	if a, b := goertzel(first, notes[0].Pitch.Freq()), goertzel(first, notes[1].Pitch.Freq()); a <= b {
		t.Errorf("the opening half sounds %s (%.5g) no louder than %s (%.5g); the notes are not in sequence", notes[0].Pitch, a, notes[1].Pitch, b)
	}

	c.read(t, c.spec.FailureDuration)
	second := c.read(t, c.spec.FailureDuration/2)
	if a, b := goertzel(second, notes[1].Pitch.Freq()), goertzel(second, notes[0].Pitch.Freq()); a <= b {
		t.Errorf("after the first note the buffer sounds %s (%.5g) no louder than %s (%.5g); the descent is missing", notes[1].Pitch, a, notes[0].Pitch, b)
	}
}

func TestTheMixerStaysInsideUnityWithFourDronesAndAPhrase(t *testing.T) {
	c := newCapture(t)
	for _, id := range []string{"one", "two", "three", "four"} {
		c.apply(t, protocol.Event{Event: protocol.SessionStarted, ID: id})
	}
	c.read(t, 3*time.Second)

	c.apply(t, protocol.Event{Event: protocol.SessionCompleted, ID: "four"})

	loudest := 0.0
	for range 20 {
		if p := peak(c.read(t, 100*time.Millisecond)); p > loudest {
			loudest = p
		}
	}
	if loudest > 1 {
		t.Errorf("peak sample %.6f leaves [-1, 1] with four drones and a phrase sounding", loudest)
	}
	if loudest == 0 {
		t.Error("four drones and a phrase produced silence")
	}
}

func TestTheSoundscapeDecaysToSilence(t *testing.T) {
	c := newCapture(t)
	c.apply(t, protocol.Event{Event: protocol.SessionStarted, ID: "one"})
	c.read(t, 3*time.Second)

	if peak(c.read(t, 100*time.Millisecond)) == 0 {
		t.Fatal("the drone never sounded")
	}

	c.apply(t, protocol.Event{Event: protocol.SessionCompleted, ID: "one"})
	c.read(t, c.release+c.spec.CompletionDuration+time.Second)

	if p := peak(c.read(t, 200*time.Millisecond)); p != 0 {
		t.Errorf("peak sample %.9f once the release envelope elapsed, want exact silence", p)
	}
}
