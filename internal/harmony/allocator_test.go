package harmony

import (
	"fmt"
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

func TestAllocatorSequentialDegrees(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	v0 := a.Acquire("s0")
	v1 := a.Acquire("s1")
	v2 := a.Acquire("s2")

	if v0.Degree != 0 {
		t.Errorf("s0: want degree 0, got %d", v0.Degree)
	}
	if v1.Degree != 1 {
		t.Errorf("s1: want degree 1, got %d", v1.Degree)
	}
	if v2.Degree != 2 {
		t.Errorf("s2: want degree 2, got %d", v2.Degree)
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
	want := scale.Degree(root, 1)
	if v.Pitch != want {
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

	for i := range MaxVoices {
		a.Acquire(fmt.Sprintf("s%d", i))
	}

	extra := a.Acquire("overflow")
	if extra.Degree != MaxVoices-1 {
		t.Errorf("capped voice: want degree %d, got %d", MaxVoices-1, extra.Degree)
	}
	wantPitch := scale.Degree(root, MaxVoices-1)
	if extra.Pitch != wantPitch {
		t.Errorf("capped voice: want pitch %v, got %v", wantPitch, extra.Pitch)
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
