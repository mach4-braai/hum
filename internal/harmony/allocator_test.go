package harmony

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func mustParsePitch(t *testing.T, s string) Pitch {
	t.Helper()
	p, err := ParsePitch(s)
	if err != nil {
		t.Fatalf("ParsePitch(%q): %v", s, err)
	}
	return p
}

func mustLookupScale(t *testing.T, name string) Scale {
	t.Helper()
	s, err := LookupScale(name)
	if err != nil {
		t.Fatalf("LookupScale(%q): %v", name, err)
	}
	return s
}

func TestAllocatorHandsOutConsonantIntervalsFirst(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	want := []string{"D2", "F3", "D4", "A3", "G3", "C4"}
	for i, name := range want {
		v := a.Acquire(fmt.Sprintf("s%d", i))
		if got := v.Pitch.String(); got != name {
			t.Errorf("voice %d sounds %s, want %s: voices arrive by interval function, not in scale order", i+1, got, name)
		}
	}
}

func TestAllocationOrderPrefersThirdsAndSixthsWhenTheScaleHasThem(t *testing.T) {
	root := mustParsePitch(t, "D2")
	a := NewAllocator(root, mustLookupScale(t, "major_pentatonic"))

	want := []string{"D2", "F#3", "B3"}
	for i, name := range want {
		if got := a.Acquire(fmt.Sprintf("s%d", i)).Pitch.String(); got != name {
			t.Errorf("voice %d sounds %s, want %s: a scale offering a third and a sixth sounds them before its octave", i+1, got, name)
		}
	}
}

func TestConcurrentVoicesAreNeverASecondApart(t *testing.T) {
	root := mustParsePitch(t, "D2")

	for _, name := range ScaleNames() {
		scale := mustLookupScale(t, name)
		a := NewAllocator(root, scale)

		var sounding []Pitch
		for i := range 3 {
			sounding = append(sounding, a.Acquire(fmt.Sprintf("s%d", i)).Pitch)
		}
		for i := range sounding {
			for j := i + 1; j < len(sounding); j++ {
				if gap := sounding[j].Midi() - sounding[i].Midi(); gap < 3 {
					t.Errorf("%s: %v and %v are %d semitones apart, which is the roughness a second produces", name, sounding[i], sounding[j], gap)
				}
			}
		}
	}
}

func TestAllocatorReleaseReuse(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	a.Acquire("s0")
	a.Acquire("s1")
	a.Acquire("s2")
	a.Release("s1")
	v := a.Acquire("s3")

	if v.Degree != 1 {
		t.Errorf("want degree 1 reused, got %d", v.Degree)
	}
	if want := mustParsePitch(t, "F3"); v.Pitch != want {
		t.Errorf("want pitch %v, got %v", want, v.Pitch)
	}
}

func TestAllocatorIdempotent(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	v1 := a.Acquire("s0")
	v2 := a.Acquire("s0")

	if v1 != v2 {
		t.Errorf("idempotent: first %+v, second %+v", v1, v2)
	}
	if a.Active() != 1 {
		t.Errorf("want 1 active voice, got %d", a.Active())
	}
}

func TestAllocatorDeterministic(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")

	run := func() []Voice {
		a := NewAllocator(root, scale)
		a.Acquire("s0")
		a.Acquire("s1")
		a.Acquire("s2")
		a.Release("s1")
		return []Voice{a.Acquire("s3")}
	}

	r1 := run()
	r2 := run()

	if r1[0].Degree != r2[0].Degree || r1[0].Pitch != r2[0].Pitch {
		t.Errorf("non-deterministic: run1=%+v run2=%+v", r1[0], r2[0])
	}
	if r1[0].Degree != 1 {
		t.Errorf("want degree 1, got %d", r1[0].Degree)
	}
}

func TestAllocatorCap(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	var highest Voice
	for i := range MaxVoices {
		highest = a.Acquire(fmt.Sprintf("s%d", i))
	}

	extra := a.Acquire("overflow")
	if extra.Degree != MaxVoices-1 {
		t.Errorf("capped voice: want degree %d, got %d", MaxVoices-1, extra.Degree)
	}
	if extra.Pitch != highest.Pitch {
		t.Errorf("capped voice: want pitch %v shared with the highest degree, got %v", highest.Pitch, extra.Pitch)
	}
	if a.Active() != MaxVoices+1 {
		t.Errorf("want %d active, got %d", MaxVoices+1, a.Active())
	}
}

func TestAllocatorCapReleaseNoCorruption(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	for i := range MaxVoices {
		a.Acquire(fmt.Sprintf("s%d", i))
	}
	a.Acquire("overflow1")
	a.Acquire("overflow2")

	a.Release("overflow1")
	a.Release("overflow2")

	if a.Active() != MaxVoices {
		t.Errorf("after releasing capped: want %d active, got %d", MaxVoices, a.Active())
	}

	a.Release("s0")
	v := a.Acquire("snew")
	if v.Degree != 0 {
		t.Errorf("after releasing real voice: want degree 0, got %d", v.Degree)
	}
}

func TestAllocatorReleaseUnknown(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	a.Release("nobody")

	if a.Active() != 0 {
		t.Errorf("want 0 active, got %d", a.Active())
	}
}

func TestAllocatorVoicesSortedByDegree(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	a.Acquire("s2")
	a.Acquire("s0")
	a.Acquire("s1")

	voices := a.Voices()
	if len(voices) != 3 {
		t.Fatalf("want 3 voices, got %d", len(voices))
	}
	for i := 1; i < len(voices); i++ {
		if voices[i].Degree < voices[i-1].Degree {
			t.Errorf("voices not sorted at index %d: %d < %d", i, voices[i].Degree, voices[i-1].Degree)
		}
	}
}

func TestAllocatorVoiceFor(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	_, ok := a.VoiceFor("missing")
	if ok {
		t.Error("VoiceFor missing: want false")
	}

	a.Acquire("s0")
	v, ok := a.VoiceFor("s0")
	if !ok {
		t.Error("VoiceFor s0: want true")
	}
	if v.Degree != 0 {
		t.Errorf("VoiceFor s0: want degree 0, got %d", v.Degree)
	}
}

func TestAllocatorRace(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("s%d", n)
			a.Acquire(id)
			a.Release(id)
		}(i)
	}
	wg.Wait()
}

func TestVoicesOrderIsDeterministicPastTheCap(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")

	order := func() []string {
		a := NewAllocator(root, scale)
		for _, id := range []string{"s09", "s03", "s11", "s01", "s14", "s07", "s02", "s13", "s05", "s10", "s04", "s12", "s08", "s06", "s15"} {
			a.Acquire(id)
		}
		ids := make([]string, 0, MaxVoices+3)
		for _, v := range a.Voices() {
			ids = append(ids, v.SessionID)
		}
		return ids
	}

	first := order()
	if len(first) <= MaxVoices {
		t.Fatalf("acquired %d voices, want more than the %d cap so shared degrees are exercised", len(first), MaxVoices)
	}
	for attempt := range 8 {
		if got := order(); !reflect.DeepEqual(got, first) {
			t.Fatalf("Voices() order = %v on attempt %d, want the stable %v; sessions sharing the capped degree must not depend on map order", got, attempt, first)
		}
	}
}

func TestVoicingLiftsHarmoniesAnOctave(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")

	want := []string{"D2", "F3", "G3", "A3", "C4", "D4"}
	for degree, name := range want {
		if got := voicing(root, scale, degree).String(); got != name {
			t.Errorf("degree %d sounds %s, want %s: harmonies sit an octave above the root", degree, got, name)
		}
	}
}

func TestVoicingStaysWithinTwoOctavesOfTheRoot(t *testing.T) {
	root := mustParsePitch(t, "D2")
	ceiling := root.Midi() + 24

	for _, name := range ScaleNames() {
		scale := mustLookupScale(t, name)
		reached := false
		for degree := range MaxVoices {
			p := voicing(root, scale, degree)
			if p.Midi() < root.Midi() || p.Midi() > ceiling {
				t.Errorf("%s degree %d sounds %v (midi %d), want within [%d, %d]", name, degree, p, p.Midi(), root.Midi(), ceiling)
			}
			reached = reached || p.Midi() == ceiling
		}
		if !reached {
			t.Errorf("%s never reaches %d, want the top harmony exactly two octaves above the root", name, ceiling)
		}
	}
}

func TestVoicingFoldsBackOnceTheScaleRunsOut(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	steps := len(scale.Intervals)

	if first, folded := voicing(root, scale, 1), voicing(root, scale, steps+1); first != folded {
		t.Errorf("degree %d sounds %v, want %v shared with degree 1", steps+1, folded, first)
	}
	if p := voicing(root, scale, -1); p != root {
		t.Errorf("negative degree sounds %v, want the root %v", p, root)
	}
}
