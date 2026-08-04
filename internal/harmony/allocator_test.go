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

	want := []string{"D2", "F2", "D4", "A3", "G3", "C4"}
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

	want := []string{"D2", "F#2", "B1"}
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
				gap := sounding[j].Midi() - sounding[i].Midi()
				if gap < 0 {
					gap = -gap
				}
				if gap < 3 {
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
	if want := mustParsePitch(t, "F2"); v.Pitch != want {
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

func TestVoicingKeepsThirdsCloseAndDropsSixthsBelowTheRoot(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")

	want := []string{"D2", "F2", "G3", "A3", "C4", "D4"}
	for degree, name := range want {
		if got := voicing(root, scale, degree).String(); got != name {
			t.Errorf("degree %d sounds %s, want %s: a third sits beside the root, every other harmony an octave above", degree, got, name)
		}
	}

	sixth := mustLookupScale(t, "major_pentatonic")
	if got := voicing(root, sixth, 4).String(); got != "B1" {
		t.Errorf("the sixth sounds %s, want B1 below the root: a sixth inverts to a third", got)
	}
}

func TestVoicingSpansAMajorThirdBelowToTwoOctavesAboveTheRoot(t *testing.T) {
	root := mustParsePitch(t, "D2")
	ceiling := root.Midi() + 24
	floor := root.Midi() - 4

	for _, name := range ScaleNames() {
		scale := mustLookupScale(t, name)
		reached := false
		for degree := range MaxVoices {
			p := voicing(root, scale, degree)
			if p.Midi() < floor || p.Midi() > ceiling {
				t.Errorf("%s degree %d sounds %v (midi %d), want within [%d, %d]", name, degree, p, p.Midi(), floor, ceiling)
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

func TestAllocationOrderExact(t *testing.T) {
	scale := mustLookupScale(t, "minor_pentatonic")
	want := []int{0, 1, 5, 3, 2, 4, 6, 7, 8, 9, 10, 11}
	if got := allocationOrder(scale); !reflect.DeepEqual(got, want) {
		t.Errorf("allocationOrder minor_pentatonic: want %v, got %v", want, got)
	}
}

func TestAllocationOrderExactForMajorScale(t *testing.T) {
	scale := mustLookupScale(t, "major")
	want := []int{0, 2, 5, 7, 4, 3, 6, 1, 8, 9, 10, 11}
	if got := allocationOrder(scale); !reflect.DeepEqual(got, want) {
		t.Errorf("allocationOrder major: want %v, got %v", want, got)
	}
}

func TestAllocationOrderLengthIs12(t *testing.T) {
	for _, name := range ScaleNames() {
		scale := mustLookupScale(t, name)
		if got := len(allocationOrder(scale)); got != 12 {
			t.Errorf("allocationOrder %s: want length 12, got %d", name, got)
		}
	}
}

func TestNewAllocatorWithMaxIntervalScale(t *testing.T) {
	root := mustParsePitch(t, "C4")
	chromatic := Scale{Intervals: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}}
	a := NewAllocator(root, chromatic)
	seen := make(map[int]bool)
	for i := range MaxVoices {
		v := a.Acquire(fmt.Sprintf("s%d", i))
		if seen[v.Degree] {
			t.Fatalf("session %d: degree %d allocated twice before cap", i, v.Degree)
		}
		seen[v.Degree] = true
	}
	if len(seen) != MaxVoices {
		t.Errorf("12-interval scale: want %d distinct degrees, got %d", MaxVoices, len(seen))
	}
}

func TestAllocatorReleaseUnknownPreservesFreeList(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	a.Release("nobody")

	v0 := a.Acquire("s0")
	v1 := a.Acquire("s1")
	if v0.Degree == v1.Degree {
		t.Errorf("after releasing unknown session: s0 and s1 both got degree %d, want distinct degrees", v0.Degree)
	}
}

func TestAllocatorFreeListMaintainsAllocationOrder(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)

	for i := range 6 {
		a.Acquire(fmt.Sprintf("s%d", i))
	}
	a.Release("s2")
	a.Release("s4")

	wantDegrees := []int{5, 2, 6, 7, 8, 9, 10, 11}
	for i, want := range wantDegrees {
		v := a.Acquire(fmt.Sprintf("t%d", i))
		if v.Degree != want {
			t.Errorf("acquire %d after releases: want degree %d, got %d", i, want, v.Degree)
		}
	}
}

func TestDegreeClassNormalizesModulo12(t *testing.T) {
	s := Scale{Intervals: []int{0, 12, 13, 5}}
	order := allocationOrder(s)
	pos := make(map[int]int, len(order))
	for i, d := range order {
		pos[d] = i
	}
	if pos[1] >= pos[3] {
		t.Errorf("degree 1 (interval 12 → class 0, rank 4) must precede degree 3 (interval 5, rank 6): pos1=%d pos3=%d", pos[1], pos[3])
	}
	if pos[3] >= pos[2] {
		t.Errorf("degree 3 (interval 5, rank 6) must precede degree 2 (interval 13 → class 1, rank 10): pos3=%d pos2=%d", pos[3], pos[2])
	}
}

func TestAllocationOrderMajorThirdBeforeMinorThird(t *testing.T) {
	s := Scale{Intervals: []int{0, 3, 4, 7}}
	order := allocationOrder(s)
	pos := make(map[int]int, len(order))
	for i, d := range order {
		pos[d] = i
	}
	if pos[2] >= pos[1] {
		t.Errorf("major third (degree 2, class 4, rank 0) must precede minor third (degree 1, class 3, rank 1): pos2=%d pos1=%d", pos[2], pos[1])
	}
}

func TestAllocatorCappedReleaseReturnsNoDegree(t *testing.T) {
	root := mustParsePitch(t, "D2")
	scale := mustLookupScale(t, "minor_pentatonic")
	a := NewAllocator(root, scale)
	for i := range MaxVoices {
		a.Acquire(fmt.Sprintf("s%d", i))
	}

	for i := range 5 {
		id := fmt.Sprintf("over%d", i)
		a.Acquire(id)
		a.Release(id)
	}

	if len(a.free) != 0 {
		t.Errorf("free list after five capped acquire and release cycles = %v, want empty: a capped session never took a degree, and returning one each time grows the list without bound", a.free)
	}
}

func TestAllocationOrderKeepsEqualConsonanceInDegreeOrder(t *testing.T) {
	doubledRoot := Scale{Intervals: []int{0, 12, 4, 7}}
	want := []int{0, 2, 1, 4, 3, 5, 6, 7, 8, 9, 10, 11}
	if got := allocationOrder(doubledRoot); !reflect.DeepEqual(got, want) {
		t.Errorf("allocationOrder with degrees 1 and 4 both sounding the root: want %v, got %v; equal consonance must keep the lower degree first", want, got)
	}
}
