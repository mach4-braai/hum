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

The engine does **not** reproduce that example. Allocation is by interval
function, so four concurrent sessions rooted at D2 sound D2, F3, D4, A3 — the
root, its third, its octave and its fifth, with every harmony lifted an octave
(see **Voicing** below). The PRD's C is the last pitch the scale has to offer and
does not appear until a sixth session arrives.

Issue #14 originally fixed allocation as the lowest free degree, which spelled
D2, F3, G3, A3. Issue #75 replaced that: two sessions holding neighbouring scale
steps sit around half a critical band apart, which is audible roughness rather
than harmony. Measured in ERB units, the minimum spacing between three
concurrent voices rose from 0.66 to 3.21 by handing out degrees in order of
interval function instead of scale order.

What made lowest-free attractive is preserved. The order is a fixed permutation
of degrees, so a released voice still returns to the pool and is handed to the
next session without re-voicing any drone still sounding, which is what `PRD.md`
§7 requires when it says completed sessions disappear.

---

## Allocation

`Allocator` owns the mapping between session IDs and musical voices. It maintains a free list of degrees (integers 0 through `MaxVoices−1`) held in **allocation order**, and `Acquire` always takes the first entry. Allocation order is degree 0, then the harmony degrees ranked by the interval each sounds above the root — thirds, then sixths, then the octave, the fifth, the fourth, the sevenths, the seconds, and the tritone last — then any remaining degrees, which duplicate pitches already in the list. Three sessions rooted at D2 on `minor_pentatonic` therefore sound D2, F3, D4 (degrees 0, 1, 5), not the neighbouring D2, F3, G3. The ranking is over interval classes, so it adapts to the scale: `major_pentatonic` sounds D2, F#3, B3, having both a third and a sixth to offer, while `minor_pentatonic` has no sixth at all and reaches for its octave second. When a session is released its degree is reinserted at its allocation-order position, and the next session to arrive reuses it without any re-voicing of the drones still sounding. This is the direct consequence of the PRD §7 requirement that "completed sessions disappear" — see the **PRD §7 allocation example** section above for why the result diverges from the four-voice voicing shown in the PRD.

`MaxVoices` is 12. When all 12 degrees are in use the allocator is at capacity: new sessions receive `Degree = MaxVoices−1` and the same pitch as that degree. They are still tracked by session ID so `hum status` can list them, and `audio.Renderer` keys an oscillator per session ID, so more than 12 concurrent sessions really do open more than 12 oscillators — what the cap bounds is the number of *distinct* pitches, which the voicing fold below lowers further to `len(Intervals) + 1`. Releasing a capped session removes it from the tracking table but returns nothing to the free pool, because no degree was consumed. Releasing any of the 12 normal voices puts that degree back, restoring the cap headroom immediately.

The allocator guards its state with a mutex. `Apply` on the engine is called from the daemon's single event goroutine, but the allocator is also exercised under `-race` in tests, so the lock is non-negotiable.

### Voicing

Degree 0 sounds the root exactly. Every degree above it keeps its scale step but
sounds one octave higher.

| degree | before | after |
|--------|--------|-------|
| 0      | D2     | D2    |
| 1      | F2     | F3    |
| 2      | G2     | G3    |
| 3      | A2     | A3    |
| 4      | C3     | C4    |
| 5      | D3     | D4    |
| 6 … 10 | F3 … D4 | unchanged |
| 11     | F4     | F3    |

`Degree` was never bounded, so the twelve voices always spanned nearly three
octaves; what crowded was the *bottom* of that span, and the bottom is where the
first few sessions land. One to five concurrent sessions — the common case — all
sounded inside the root's own octave, a minor third and a fourth apart at 73 Hz
where a critical band is around 100 Hz wide, which is the definition of
roughness. Lifting them puts an octave of air between the bass anchor and the
harmonies.

The step folds back into `1 … len(Intervals)` once it runs past the scale, which
makes `root + 24` semitones the exact ceiling: on `minor_pentatonic` degree 5
sounds D4, and degree 6 shares F3 with degree 1. The top harmony of every
built-in scale lands exactly two octaves above the root.

The root's own register is `music.octave`, defaulting to 3, so the pitches a
default install has to offer are D3, F4, G4, A4, C5, D5, listed by degree. The
order sessions receive them in is **Allocation** above, not this one: D3, F4,
D5, A4, G4, C5. The table is rooted at D2 because that is what the change to
lifted voicing replaced; both the register and the lift are visible in
`hum status`.

The price is shared pitches beyond `len(Intervals) + 1` concurrent sessions —
six voices on a pentatonic, eight on a seven-note scale — where the old mapping
kept all twelve distinct by climbing to F4. That is deliberate: three concurrent
voices is roughly the limit a listener can denumerate anyway, so audible
separation low down matters more than a unique note for the seventh simultaneous
session.

`Scale.Degree` is untouched by this: it remains a general scale function whose
`n` is a plain degree index, and the voicing rule lives in the allocator.

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

**`DefaultPhraseSpec` is not what a default install hears.** The daemon builds its
engine from `theme.PhraseSpec()`, and the built-in `minimal` theme sets
`completion_octaves: 1`, `completion_duration: 0.2`, `failure_duration: 1.2` and
`failure_gain: 0.35`. So a default install completes **one** octave above the
drone — D3 becomes D4, not D5. `DefaultPhraseSpec` is the fallback for a theme
that names none of them, and every number in this section is the engine's
default, not the audible one. `e2e/buffer_test.go` asserts against the theme's
spec for that reason.

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
