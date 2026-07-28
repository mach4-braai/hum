package harmony

import (
	"fmt"
	"math"
	"strconv"
	"unicode"
)

type Pitch struct {
	Class  int
	Octave int
}

var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

func ParseNoteClass(s string) (int, error) {
	if len(s) == 0 || len(s) > 2 {
		return 0, fmt.Errorf("invalid note class %q", s)
	}
	letter := unicode.ToUpper(rune(s[0]))
	var base int
	switch letter {
	case 'C':
		base = 0
	case 'D':
		base = 2
	case 'E':
		base = 4
	case 'F':
		base = 5
	case 'G':
		base = 7
	case 'A':
		base = 9
	case 'B':
		base = 11
	default:
		return 0, fmt.Errorf("invalid note class %q", s)
	}
	if len(s) == 1 {
		return base, nil
	}
	switch s[1] {
	case '#':
		return (base + 1) % 12, nil
	case 'b':
		return (base + 11) % 12, nil
	default:
		return 0, fmt.Errorf("invalid note class %q", s)
	}
}

func ParsePitch(s string) (Pitch, error) {
	if len(s) == 0 {
		return Pitch{}, fmt.Errorf("invalid pitch %q: empty string", s)
	}

	noteEnd := 1
	if len(s) > 1 && (s[1] == '#' || s[1] == 'b') {
		noteEnd = 2
	}
	if noteEnd >= len(s) {
		return Pitch{}, fmt.Errorf("invalid pitch %q: missing octave", s)
	}

	class, err := ParseNoteClass(s[:noteEnd])
	if err != nil {
		return Pitch{}, fmt.Errorf("invalid pitch %q: %w", s, err)
	}

	octave, err := strconv.Atoi(s[noteEnd:])
	if err != nil {
		return Pitch{}, fmt.Errorf("invalid pitch %q: invalid octave %q", s, s[noteEnd:])
	}
	if octave < -1 || octave > 9 {
		return Pitch{}, fmt.Errorf("invalid pitch %q: octave %d out of range [-1, 9]", s, octave)
	}

	return Pitch{Class: class, Octave: octave}, nil
}

func (p Pitch) String() string {
	return fmt.Sprintf("%s%d", noteNames[p.Class], p.Octave)
}

func (p Pitch) Midi() int {
	return (p.Octave+1)*12 + p.Class
}

func (p Pitch) Freq() float64 {
	return 440.0 * math.Pow(2, float64(p.Midi()-69)/12.0)
}

func (p Pitch) Transpose(semitones int) Pitch {
	midi := p.Midi() + semitones
	class := midi % 12
	if class < 0 {
		class += 12
	}
	octave := (midi-class)/12 - 1
	return Pitch{Class: class, Octave: octave}
}
