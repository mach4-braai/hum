package harmony

import (
	"math"
	"testing"
)

func TestParseNoteClass(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"C", 0, false},
		{"D", 2, false},
		{"E", 4, false},
		{"F", 5, false},
		{"G", 7, false},
		{"A", 9, false},
		{"B", 11, false},
		{"c", 0, false},
		{"d", 2, false},
		{"C#", 1, false},
		{"D#", 3, false},
		{"F#", 6, false},
		{"G#", 8, false},
		{"A#", 10, false},
		{"Cb", 11, false},
		{"Db", 1, false},
		{"Eb", 3, false},
		{"Fb", 4, false},
		{"Gb", 6, false},
		{"Ab", 8, false},
		{"Bb", 10, false},
		{"E#", 5, false},
		{"B#", 0, false},
		{"", 0, true},
		{"H", 0, true},
		{"C##", 0, true},
		{"D!", 0, true},
		{"DDD", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseNoteClass(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseNoteClass(%q): expected error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseNoteClass(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseNoteClass(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParsePitch(t *testing.T) {
	cases := []struct {
		in      string
		class   int
		octave  int
		wantErr bool
	}{
		{"C4", 0, 4, false},
		{"D2", 2, 2, false},
		{"F#3", 6, 3, false},
		{"Bb1", 10, 1, false},
		{"A4", 9, 4, false},
		{"G#8", 8, 8, false},
		{"C-1", 0, -1, false},
		{"G9", 7, 9, false},
		{"B9", 0, 0, true},
		{"", 0, 0, true},
		{"H4", 0, 0, true},
		{"D", 0, 0, true},
		{"D99", 0, 0, true},
		{"D10", 0, 0, true},
		{"D-2", 0, 0, true},
		{"Da", 0, 0, true},
		{"4", 0, 0, true},
		{"#4", 0, 0, true},
	}
	for _, tc := range cases {
		got, err := ParsePitch(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParsePitch(%q): expected error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePitch(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got.Class != tc.class || got.Octave != tc.octave {
			t.Errorf("ParsePitch(%q) = {%d, %d}, want {%d, %d}", tc.in, got.Class, got.Octave, tc.class, tc.octave)
		}
	}
}

func TestPitchString(t *testing.T) {
	cases := []struct {
		p    Pitch
		want string
	}{
		{Pitch{0, 4}, "C4"},
		{Pitch{9, 4}, "A4"},
		{Pitch{2, 2}, "D2"},
		{Pitch{6, 3}, "F#3"},
		{Pitch{10, 1}, "A#1"},
		{Pitch{11, 9}, "B9"},
	}
	for _, tc := range cases {
		got := tc.p.String()
		if got != tc.want {
			t.Errorf("Pitch{%d,%d}.String() = %q, want %q", tc.p.Class, tc.p.Octave, got, tc.want)
		}
	}
}

func TestParsePitchStringRoundTrip(t *testing.T) {
	sharps := []string{"C4", "C#4", "D4", "D#4", "E4", "F4", "F#4", "G4", "G#4", "A4", "A#4", "B4"}
	for _, s := range sharps {
		p, err := ParsePitch(s)
		if err != nil {
			t.Fatalf("ParsePitch(%q): %v", s, err)
		}
		got := p.String()
		if got != s {
			t.Errorf("round-trip %q -> String() = %q", s, got)
		}
	}
}

func TestFlatNormalizesToSharp(t *testing.T) {
	p, err := ParsePitch("Bb1")
	if err != nil {
		t.Fatalf("ParsePitch(\"Bb1\"): %v", err)
	}
	if got := p.String(); got != "A#1" {
		t.Errorf("Bb1 normalised to %q, want A#1", got)
	}
}

func TestAccidentalCarriesAcrossTheOctave(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"B#3", "C4"},
		{"Cb4", "B3"},
		{"B#0", "C1"},
		{"Cb0", "B-1"},
		{"A#3", "A#3"},
		{"Db4", "C#4"},
	}
	for _, c := range cases {
		p, err := ParsePitch(c.in)
		if err != nil {
			t.Fatalf("ParsePitch(%q): %v", c.in, err)
		}
		if got := p.String(); got != c.want {
			t.Errorf("ParsePitch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePitchRejectsOutsideMidiRange(t *testing.T) {
	for _, s := range []string{"G#9", "A9", "B9", "Cb-1"} {
		if p, err := ParsePitch(s); err == nil {
			t.Errorf("ParsePitch(%q) = %v (midi %d), want an error", s, p, p.Midi())
		}
	}
	for _, s := range []string{"C-1", "G9"} {
		p, err := ParsePitch(s)
		if err != nil {
			t.Fatalf("ParsePitch(%q) at the MIDI boundary: %v", s, err)
		}
		if p.Midi() < MinMidi || p.Midi() > MaxMidi {
			t.Errorf("ParsePitch(%q).Midi() = %d, want within [%d, %d]", s, p.Midi(), MinMidi, MaxMidi)
		}
	}
}

func TestMidi(t *testing.T) {
	cases := []struct {
		p    Pitch
		want int
	}{
		{Pitch{0, 4}, 60},
		{Pitch{9, 4}, 69},
		{Pitch{0, 0}, 12},
		{Pitch{0, -1}, 0},
		{Pitch{11, 9}, 131},
	}
	for _, tc := range cases {
		got := tc.p.Midi()
		if got != tc.want {
			t.Errorf("%v.Midi() = %d, want %d", tc.p, got, tc.want)
		}
	}
}

func TestFreq(t *testing.T) {
	a4, _ := ParsePitch("A4")
	if got := a4.Freq(); got != 440.0 {
		t.Errorf("A4.Freq() = %v, want exactly 440.0", got)
	}

	d2, _ := ParsePitch("D2")
	want := 73.41619197935188
	if got := d2.Freq(); math.Abs(got-want) > 1e-9 {
		t.Errorf("D2.Freq() = %.14f, want %.14f (delta > 1e-9)", got, want)
	}
}

func TestTransposeOctave(t *testing.T) {
	starts := []string{"C2", "D2", "F#3", "A4", "G#5"}
	for _, s := range starts {
		p, err := ParsePitch(s)
		if err != nil {
			t.Fatalf("ParsePitch(%q): %v", s, err)
		}
		up := p.Transpose(12)
		if math.Abs(up.Freq()-p.Freq()*2) > 1e-9 {
			t.Errorf("%s: Transpose(12).Freq() = %v, want %v*2", s, up.Freq(), p.Freq())
		}
	}
}

func TestTransposeNegative(t *testing.T) {
	a4, _ := ParsePitch("A4")
	a3 := a4.Transpose(-12)
	if a3.String() != "A3" {
		t.Errorf("A4.Transpose(-12) = %q, want A3", a3)
	}
	if math.Abs(a3.Freq()-220.0) > 1e-9 {
		t.Errorf("A3.Freq() = %v, want 220.0", a3.Freq())
	}
}

func TestTransposeMultiOctave(t *testing.T) {
	c4, _ := ParsePitch("C4")
	c6 := c4.Transpose(24)
	if c6.String() != "C6" {
		t.Errorf("C4.Transpose(24) = %q, want C6", c6)
	}
	if math.Abs(c6.Freq()-c4.Freq()*4) > 1e-9 {
		t.Errorf("C6.Freq() = %v, want C4.Freq()*4", c6.Freq())
	}
}

func TestTransposeNormalizesClass(t *testing.T) {
	cases := []struct {
		start     string
		semitones int
		want      string
	}{
		{"C4", 1, "C#4"},
		{"B4", 1, "C5"},
		{"C4", -1, "B3"},
		{"D2", 10, "C3"},
		{"F#3", 6, "C4"},
	}
	for _, tc := range cases {
		p, _ := ParsePitch(tc.start)
		got := p.Transpose(tc.semitones).String()
		if got != tc.want {
			t.Errorf("%s.Transpose(%d) = %q, want %q", tc.start, tc.semitones, got, tc.want)
		}
	}
}
func TestTransposeNegativeMidi(t *testing.T) {
	p := Pitch{Class: 0, Octave: -1}
	got := p.Transpose(-3)
	if got.String() != "A-2" {
		t.Errorf("C-1.Transpose(-3) = %q, want A-2", got.String())
	}
	if got.Octave != -2 {
		t.Errorf("C-1.Transpose(-3).Octave = %d, want -2", got.Octave)
	}
}
