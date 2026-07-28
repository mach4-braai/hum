package harmony

import (
	"sort"
	"strings"
	"testing"
)

func TestLookupScaleValid(t *testing.T) {
	names := ScaleNames()
	for _, name := range names {
		s, err := LookupScale(name)
		if err != nil {
			t.Errorf("LookupScale(%q): unexpected error: %v", name, err)
			continue
		}
		if s.Name != name {
			t.Errorf("LookupScale(%q).Name = %q, want %q", name, s.Name, name)
		}
		if len(s.Intervals) == 0 {
			t.Errorf("LookupScale(%q): empty intervals", name)
		}
	}
}

func TestLookupScaleNormalization(t *testing.T) {
	variants := []string{
		"minor_pentatonic",
		"minor-pentatonic",
		"Minor Pentatonic",
		"MINOR_PENTATONIC",
		"Minor-Pentatonic",
		"minor pentatonic",
	}
	first, err := LookupScale(variants[0])
	if err != nil {
		t.Fatalf("LookupScale(%q): %v", variants[0], err)
	}
	for _, v := range variants[1:] {
		s, err := LookupScale(v)
		if err != nil {
			t.Errorf("LookupScale(%q): unexpected error: %v", v, err)
			continue
		}
		if s.Name != first.Name {
			t.Errorf("LookupScale(%q).Name = %q, want %q", v, s.Name, first.Name)
		}
		if len(s.Intervals) != len(first.Intervals) {
			t.Errorf("LookupScale(%q): interval length mismatch", v)
		}
	}
}

func TestLookupScaleUnknown(t *testing.T) {
	_, err := LookupScale("chromatic")
	if err == nil {
		t.Fatal("LookupScale(\"chromatic\"): expected error, got nil")
	}
	msg := err.Error()
	for _, name := range ScaleNames() {
		if !strings.Contains(msg, name) {
			t.Errorf("error message missing valid name %q: %s", name, msg)
		}
	}
}

func TestScaleNamesIsSorted(t *testing.T) {
	names := ScaleNames()
	if !sort.StringsAreSorted(names) {
		t.Errorf("ScaleNames() not sorted: %v", names)
	}
}

func TestScaleNamesMutationSafe(t *testing.T) {
	before := ScaleNames()
	after := ScaleNames()
	before[0] = "mutated"
	if after[0] == "mutated" {
		t.Error("ScaleNames() returns aliased slice; caller mutation corrupted subsequent call")
	}
}

func TestScaleIntervalsMutationSafe(t *testing.T) {
	s1, _ := LookupScale("minor_pentatonic")
	orig := make([]int, len(s1.Intervals))
	copy(orig, s1.Intervals)

	s1.Intervals[0] = 99

	s2, _ := LookupScale("minor_pentatonic")
	for i, v := range s2.Intervals {
		if v != orig[i] {
			t.Errorf("mutating returned Scale.Intervals corrupted built-in table at index %d: got %d, want %d", i, v, orig[i])
		}
	}
}

func TestScaleIntervalsCopied(t *testing.T) {
	s, _ := LookupScale("major")
	s.Intervals[0] = 99
	s2, _ := LookupScale("major")
	if s2.Intervals[0] == 99 {
		t.Error("LookupScale returns aliased Intervals slice")
	}
}

func TestDegreeConsecutiveMinorPentatonic(t *testing.T) {
	scale, _ := LookupScale("minor_pentatonic")
	root, _ := ParsePitch("D2")

	cases := []struct {
		n    int
		want string
	}{
		{0, "D2"},
		{1, "F2"},
		{2, "G2"},
		{3, "A2"},
	}
	for _, tc := range cases {
		got := scale.Degree(root, tc.n)
		if got.String() != tc.want {
			t.Errorf("Degree(D2, %d) = %q, want %q", tc.n, got.String(), tc.want)
		}
	}
}

func TestDegreeOctaveWrap(t *testing.T) {
	scale, _ := LookupScale("minor_pentatonic")
	root, _ := ParsePitch("D2")

	got := scale.Degree(root, 5)
	if got.String() != "D3" {
		t.Errorf("Degree(D2, 5) = %q, want D3 (octave wrap)", got.String())
	}
	if got.Octave != 3 {
		t.Errorf("Degree(D2, 5).Octave = %d, want 3", got.Octave)
	}
}

func TestDegreeNegative(t *testing.T) {
	scale, _ := LookupScale("minor_pentatonic")
	root, _ := ParsePitch("D2")

	got := scale.Degree(root, -1)
	if got.String() != "C2" {
		t.Errorf("Degree(D2, -1) = %q, want C2", got.String())
	}
}

func TestDegreeNegativeOctaveWrap(t *testing.T) {
	scale, _ := LookupScale("minor_pentatonic")
	root, _ := ParsePitch("D2")

	got := scale.Degree(root, -5)
	if got.String() != "D1" {
		t.Errorf("Degree(D2, -5) = %q, want D1 (negative octave wrap)", got.String())
	}
}

func TestPRDAllocationExample(t *testing.T) {
	scale, _ := LookupScale("minor_pentatonic")
	root, _ := ParsePitch("D2")

	sequence := []int{0, 1, 3, 4}
	want := []string{"D2", "F2", "A2", "C3"}

	for i, deg := range sequence {
		got := scale.Degree(root, deg)
		if got.String() != want[i] {
			t.Errorf("PRD example: Degree(D2, %d) = %q, want %q", deg, got.String(), want[i])
		}
	}
}

func TestBuiltInScaleIntervals(t *testing.T) {
	cases := []struct {
		name      string
		intervals []int
	}{
		{"minor_pentatonic", []int{0, 3, 5, 7, 10}},
		{"major_pentatonic", []int{0, 2, 4, 7, 9}},
		{"dorian", []int{0, 2, 3, 5, 7, 9, 10}},
		{"aeolian", []int{0, 2, 3, 5, 7, 8, 10}},
		{"major", []int{0, 2, 4, 5, 7, 9, 11}},
		{"lydian", []int{0, 2, 4, 6, 7, 9, 11}},
		{"phrygian", []int{0, 1, 3, 5, 7, 8, 10}},
	}
	for _, tc := range cases {
		s, err := LookupScale(tc.name)
		if err != nil {
			t.Fatalf("LookupScale(%q): %v", tc.name, err)
		}
		if len(s.Intervals) != len(tc.intervals) {
			t.Errorf("%s: got %d intervals, want %d", tc.name, len(s.Intervals), len(tc.intervals))
			continue
		}
		for i, v := range s.Intervals {
			if v != tc.intervals[i] {
				t.Errorf("%s: interval[%d] = %d, want %d", tc.name, i, v, tc.intervals[i])
			}
		}
	}
}

func TestDegreeWrapsByOctavePastScaleLength(t *testing.T) {
	scale, _ := LookupScale("major")
	root, _ := ParsePitch("C4")
	cases := []struct {
		n    int
		want string
	}{
		{7, "C5"},
		{14, "C6"},
	}
	for _, tc := range cases {
		got := scale.Degree(root, tc.n)
		if got.String() != tc.want {
			t.Errorf("Degree(C4, %d) on major = %q, want %q", tc.n, got.String(), tc.want)
		}
	}
}
