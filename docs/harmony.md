# Harmony

## Pitch

### Tuning basis

Hum uses 12-tone equal temperament (12-TET) with A4 = 440 Hz as the sole tuning anchor.

Every pitch is represented as a `Pitch{Class int, Octave int}` where `Class` is 0–11 (C=0 through B=11) and `Octave` uses scientific pitch notation: middle C is C4.

MIDI note number: `(Octave + 1) * 12 + Class`. This places C4 at MIDI 60 and A4 at MIDI 69.

Frequency in hertz: `440 × 2^((Midi − 69) / 12)`.

Valid octave range is −1 to 9, covering the full MIDI spectrum (MIDI 0–127, i.e. C−1 through G9).

### Enharmonic normalisation

Flats are immediately normalised to their sharp enharmonic equivalent on parse. `Bb1` becomes `A#1`; `Db3` becomes `C#3`. `String()` always renders sharps, so a parse/format round-trip is lossless for sharps and normalising for flats. There is a single canonical representation for every pitch.

This removes the need to treat two names as the same pitch anywhere downstream: the registry, the harmony engine, and the audio engine all see class integers, never string spellings.

### Accidental table

| Natural | Class | Sharp | Class | Flat of | Class |
|---------|-------|-------|-------|---------|-------|
| C       | 0     | C#    | 1     | Db      | 1     |
| D       | 2     | D#    | 3     | Eb      | 3     |
| E       | 4     | E#    | 5     | Fb      | 4     |
| F       | 5     | F#    | 6     | Gb      | 6     |
| G       | 7     | G#    | 8     | Ab      | 8     |
| A       | 9     | A#    | 10    | Bb      | 10    |
| B       | 11    | B#    | 0     | Cb      | 11    |

---

## Scales

### Built-in scale interval table

Intervals are ascending semitone offsets from the root within one octave.

| Scale             | Intervals                | Notes |
|-------------------|--------------------------|-------|
| aeolian           | 0 2 3 5 7 8 10           | 7     |
| dorian            | 0 2 3 5 7 9 10           | 7     |
| lydian            | 0 2 4 6 7 9 11           | 7     |
| major             | 0 2 4 5 7 9 11           | 7     |
| major_pentatonic  | 0 2 4 7 9                | 5     |
| minor_pentatonic  | 0 3 5 7 10               | 5     |
| phrygian          | 0 1 3 5 7 8 10           | 7     |

`LookupScale` normalises the name to lowercase and treats hyphens and spaces as underscores, so `"Minor Pentatonic"`, `"minor-pentatonic"`, and `"minor_pentatonic"` all resolve. Unknown names return an error that lists every valid name.

`ScaleNames()` returns a sorted copy of the table keys. `LookupScale` returns a `Scale` whose `Intervals` slice is an independent copy; neither return value aliases the built-in table, so callers may freely mutate them.

### Degree wrapping

`Degree(root Pitch, n int) Pitch` maps an unbounded integer `n` to a pitch in the scale:

- Divide `n` by the number of scale notes using floor division (Go remainder adjusted for negative `n`).
- The quotient is the number of full octaves to shift; the remainder selects the interval within the current octave.
- `semitones = Intervals[idx] + octaves × 12`; the result is `root.Transpose(semitones)`.

For `minor_pentatonic` (5 notes) rooted at D2:

| n  | idx | octaves | semitones | pitch |
|----|-----|---------|-----------|-------|
| 0  | 0   | 0       | 0         | D2    |
| 1  | 1   | 0       | 3         | F2    |
| 2  | 2   | 0       | 5         | G2    |
| 3  | 3   | 0       | 7         | A2    |
| 4  | 4   | 0       | 10        | C3    |
| 5  | 0   | 1       | 12        | D3    |
| −1 | 4   | −1      | −2        | C2    |
| −5 | 0   | −1      | −12       | D1    |

### PRD §7 allocation example — deliberate discrepancy

PRD §7 shows four simultaneous work sessions allocated D2, F2, A2, C3 against root D and scale `minor_pentatonic`.

Consecutive scale degrees 0, 1, 2, 3 yield D2, F2, **G2**, **A2** — the fourth is G, not A, and C3 does not appear. The PRD example is not consecutive degrees.

The engine allocates voices at degrees **0, 1, 3, 4**, which produces D2 (root), F2 (minor third), A2 (perfect fifth), C3 (minor seventh). This is a Dm7 chord stacked across the voice ensemble, maximising harmonic consonance. Skipping degree 2 (G, the perfect fourth) avoids the clash between the fourth and the major third heard in open voicings.

The allocation sequence 0, 1, 3, 4, 5, 7, 8, … (skipping every second degree after the root-and-third pair) is the engine's responsibility, not encoded in this package. `internal/harmony` provides `Degree` for arbitrary `n`; the engine chooses which `n` values to use. This file records the intent so the discrepancy between the PRD example and consecutive degrees is a documented decision rather than a bug.
