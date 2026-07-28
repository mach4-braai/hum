# Harmony

## Pitch

### Tuning basis

Hum uses 12-tone equal temperament (12-TET) with A4 = 440 Hz as the sole tuning anchor.

Every pitch is represented as a `Pitch{Class int, Octave int}` where `Class` is 0–11 (C=0 through B=11) and `Octave` uses scientific pitch notation: middle C is C4.

MIDI note number: `(Octave + 1) * 12 + Class`. This places C4 at MIDI 60 and A4 at MIDI 69.

Frequency in hertz: `440 × 2^((Midi − 69) / 12)`.

Valid octave range is −1 to 9, and a parsed pitch must land inside MIDI 0–127
(C−1 through G9). `A9` and `B9` name real frequencies but fall outside MIDI, so
`ParsePitch` rejects them rather than returning a note the wire format cannot
describe. `Transpose` is pure arithmetic and is *not* range-checked: clamping it
would break octave doubling, and the engine only transposes within the scale it
allocated from.

### Enharmonic normalisation

Parsing canonicalises. Flats become their sharp equivalent — `Bb1` becomes `A#1`,
`Db3` becomes `C#3` — and an accidental that crosses a letter boundary also moves
the octave, so `B#3` becomes `C4`. `String()` always renders the canonical sharp
spelling, so a round-trip is lossless only for input that was already canonical;
for anything else it is normalising, by design. Every pitch has exactly one
printed form.

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

An accidental that crosses a letter boundary carries the octave with it, because
the octave number belongs to the letter name, not to the resulting pitch class.
`B#3` is the semitone above `B3`, so it parses as `C4`, not `C3`; `Cb4` is the
semitone below `C4`, so it parses as `B3`. The class column above is what
`ParseNoteClass` reports; it cannot express the carry, which is correct for its
only caller, since `music.root` in the config file is a bare class with no
octave.

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

The engine does **not** reproduce that example. Issue #14 fixes allocation as the
lowest free degree, so four concurrent sessions rooted at D2 sound D2, F2, G2,
A2 — consecutive degrees. Lowest-free is what makes allocation deterministic and
makes a released pitch immediately reusable, which `PRD.md` §7 also requires when
it says completed sessions disappear.

Degrees 0, 1, 3, 4 would spell the PRD's Dm7 voicing, and `Degree` supports any
`n` a future policy might want, but choosing pitches by chord function rather
than by availability means a released voice cannot be handed to the next session
without re-voicing every sounding drone. That is a Phase 2 question. Until then
the divergence from the §7 example is recorded here as a decision.

---

## Allocation

`Allocator` owns the mapping between session IDs and musical voices. It maintains a sorted free list of degrees (integers 0 through `MaxVoices−1`). `Acquire` always takes the smallest available degree, so sessions assigned in order receive consecutive scale degrees: three sessions rooted at D2 on `minor_pentatonic` sound D2, F2, G2 (degrees 0, 1, 2). When a session is released that degree returns immediately to the free pool; the next session to arrive reuses it without any re-voicing of the drones still sounding. This is the direct consequence of the PRD §7 requirement that "completed sessions disappear" — see the **PRD §7 allocation example** section above for why consecutive degrees diverge from the four-voice voicing shown in the PRD.

`MaxVoices` is 12. When all 12 degrees are in use the allocator is at capacity: new sessions receive `Degree = MaxVoices−1` and the same pitch as the highest allocated voice. They are still tracked by session ID so `hum status` can list them; the audio mixer, however, sees at most 12 distinct oscillator pitches. Releasing a capped session removes it from the tracking table but returns nothing to the free pool, because no degree was consumed. Releasing any of the 12 normal voices puts that degree back, restoring the cap headroom immediately.

The allocator guards its state with a mutex. `Apply` on the engine is called from the daemon's single event goroutine, but the allocator is also exercised under `-race` in tests, so the lock is non-negotiable.

---

## Expression

`Expression` has three fields, each clamped to 0–1:

| Field       | Meaning                          |
|-------------|----------------------------------|
| `Intensity` | harmonic richness / brightness   |
| `Tremolo`   | depth of amplitude modulation    |
| `Width`     | stereo spread                    |

`Intensity` and `Tremolo` both derive from a per-session **activity score** maintained by an exponential moving average (EMA). Each `session.updated` event adds 1.0 to the score; between events the score decays continuously with a **half-life of 5 seconds** — the score halves every 5 s with no further updates.

The decay constant is `λ = ln(2) / 5`. When a new event arrives at time `t` after the previous event at time `t₀`:

```
score = score × exp(−λ × (t − t₀)) + 1.0
```

The score is normalised to the range 0–1 by dividing by `exprIntensityCap = 10.0` and clamping: ten events arriving simultaneously (zero elapsed time between them) produce `Intensity = 1.0`.

Rate rather than count is used because an unbounded counter would brighten a long-running session indefinitely, making a three-hour session indistinguishable from a suddenly very active one. The EMA decays to near-zero roughly 35 seconds after the last update regardless of history.

`Width` is driven by the optional integer metadata key `agents`. Absent or non-integer metadata is treated as `agents = 1`, which yields `Width = 0`. Each additional agent above 1 raises Width linearly; `agents = 21` saturates Width at 1.0. The normaliser is `exprAgentsCap = 20.0`:

```
Width = min((agents − 1) / 20.0, 1.0)
```

A `session.updated` event updates the activity score only. It never changes `Voice.Pitch` or `Voice.Degree`.

Time is read through the package-level seam `var now = time.Now` in `expression.go`. Tests stage this seam and restore it in `t.Cleanup`; no test that stages the seam calls `t.Parallel`.

---

## Phrases

`Apply` emits phrases when a `ChangeEnded` event arrives. The voice is looked up before release so the phrase always carries the originating pitch.

### Completion

One note: the session pitch transposed up `CompletionOctaves × 12` semitones (default 2 octaves). `DefaultPhraseSpec` sets `CompletionDuration = 500 ms` and `CompletionGain = 0.8`.

**PRD §8 discrepancy.** The §8 prose reads "one or two octaves higher"; the §8 example shows a D2 session with a D5 completion note — three octaves, not two. The engine implements two octaves, following the prose. This discrepancy is recorded here rather than silently resolved.

### Failure

Two notes in sequence: the session pitch at offset 0, then the session pitch transposed by `FailureInterval` semitones (default −3, a descending minor third) at offset `FailureDuration`. This creates a recognisable descending cadence that is neither alarming nor easily confused with completion. `DefaultPhraseSpec` sets `FailureDuration = 800 ms` and `FailureGain = 0.5`, making the failure phrase longer and quieter than completion. "No sharp attack" is the renderer's responsibility; this package emits abstract notes, not waveforms.

### Cancellation

`PhraseCancelled` exists as a seam for themes that want an audible cancellation sound, but `PhraseSpec.CancelledSounds` defaults to `false`. When false, cancellation is silent: the drone fades and no phrase is emitted. Silence is the conservative default — inventing a third audible cadence risks users mistaking it for failure, exactly the confusion PRD §9 warns against. The choice is recorded in the theme file, not hardcoded.

### Why release happens after phrase construction

`buildPhrases` reads the voice's pitch before `Release` removes it from the allocator. If release happened first, a completion phrase for the last active session would have no voice to read. The ordering is: look up voice → build phrase → release voice → update state.

### Single-goroutine contract

`Engine.Apply` is called exclusively from the daemon's single event goroutine. The engine carries no internal mutex; concurrent calls would race. The allocator inside the engine has its own mutex for the `-race` test coverage required by issue #14, but the engine itself is not safe for concurrent use.

### Retune

`Retune` replaces the root pitch and scale and rebuilds the allocator from scratch. It refuses with `ErrRetuneBusy` when `alloc.Active() > 0`. The daemon only calls `Retune` on a project switch while no sessions are active, so this error indicates a protocol violation rather than a normal operating condition.
