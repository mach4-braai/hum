# Theme Reference

## File format

Theme files are YAML. Every key a waveform uses must be present; unknown keys are
an error, and so is a key belonging to the other waveform.

```yaml
name: minimal
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 0.5
  harmonic: 0.15
  tremolo_hz: 5.0
  detune_cents: 8.0
phrases:
  completion_octaves: 1
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
```

The `strings` waveform adds six keys to `drone` and drops `harmonic` to 0:

```yaml
name: orchestra
waveform: strings
drone:
  attack: 2.5
  release: 3.0
  gain: 0.5
  harmonic: 0.0
  tremolo_hz: 4.0
  detune_cents: 8.0
  partials: 12
  cutoff_hz: 1500.0
  brightness_octaves: 0.8
  ensemble_voices: 3
  ensemble_cents: 7.0
  ensemble_drift_hz: 0.15
phrases:
  completion_octaves: 1
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
```

## Key reference

### Top level

| Key        | Type   | Description                                        |
|------------|--------|----------------------------------------------------|
| `name`     | string | Non-empty identifier; must match the file stem.    |
| `waveform` | string | Oscillator shape: `sine` or `strings`. |

### `drone`

The ambient layer that persists for the lifetime of an active session.

| Key            | Type  | Unit    | Range   | Description                                                       |
|----------------|-------|---------|---------|-------------------------------------------------------------------|
| `attack`       | float | seconds | (0, 60] | Fade-in time. Values below ~1 s will click on most DAC hardware. |
| `release`      | float | seconds | (0, 60] | Fade-out time. Same guidance as attack.                           |
| `gain`         | float | linear  | [0, 1]  | Amplitude of the fundamental. `NaN` is rejected.                 |
| `harmonic`     | float | linear  | [0, 1]  | Amplitude of the second harmonic (2×fundamental). `waveform: sine` only, and it must be 0 for `strings`. `NaN` is rejected. |
| `tremolo_hz`   | float | Hz      | [0, 20] | Rate of the amplitude tremolo. 0 disables tremolo. Above 20 Hz it stops reading as tremolo and starts sounding like distortion. |
| `detune_cents` | float | cents   | [0, 100] | Per-voice detuning applied for stereo width. A whole semitone is the ceiling; `NaN` would poison the frequency arithmetic and silence the voice. |

### `drone`, `waveform: strings` only

These keys configure the partial stack, its ensemble and its filter. A `sine`
theme must leave every one of them unset, and a `strings` theme must leave
`harmonic` at 0. Mixing them is a validation error rather than a silently ignored
key: `harmonic` names the amplitude of one added partial, which a stack does not
have, and `partials` names a stack a sine does not have.

| Key                  | Type  | Unit    | Range     | Description                                                       |
|----------------------|-------|---------|-----------|-------------------------------------------------------------------|
| `partials`           | int   | count   | [2, 32]   | Partials in the stack, at 1/n amplitude. 12 reads as a string section; 4 is closer to a soft pad. Bands above the sample rate's limit are thinned automatically, so a high value costs nothing at high pitch. |
| `cutoff_hz`          | float | Hz      | (0, 20000] | Low-pass cutoff at `Intensity = 0`. 1.5 kHz takes the buzz off a 12-partial stack without dulling the fundamental. Clamped to 45% of the sample rate at render time, because this package cannot see the sample rate. |
| `brightness_octaves` | float | octaves | [0, 6]    | How far `Intensity` opens the cutoff: `fc = cutoff_hz × 2^(Intensity × brightness_octaves)`. 0 fixes the filter. `NaN` is rejected. |
| `ensemble_voices`    | int   | count   | [1, 4]    | Detuned copies of the stack per channel. 1 is a soloist and has no movement; 3 is a section. The ceiling is 4 because each copy costs two table lookups per frame in the audio callback. |
| `ensemble_cents`     | float | cents   | [0, 100]  | Width of the detune window the copies are spread across. 7 cents gives slow chorusing; past ~25 it reads as out of tune. `NaN` is rejected. |
| `ensemble_drift_hz`  | float | Hz      | [0, 2]    | Rate of the per-copy drift LFO that keeps the detuning moving. 0 freezes the copies at fixed offsets, which sounds like a chorus pedal rather than players. `NaN` is rejected. |

**Why a stack rather than more harmonics.** One sine plus one even partial reads
as an organ stop or a test tone. Odd and even partials at 1/n are the spectrum of
a sawtooth, which a low-pass shapes into something with a body. The ensemble is
what stops it sounding like one synthesizer: two or three copies a few cents
apart, each drifting independently, is the difference between one player and a
section. `docs/audio.md` § Oscillator has the synthesis detail and the cost.

**Attack and release rationale.** `PRD.md` §6 specifies that the drone should fade in and fade out rather than cut on and off abruptly. An abrupt gain change produces an audible click on every consumer DAC. 2.5 s and 3.0 s are long enough to be imperceptible on a typical coding session timeline while remaining responsive on the scale of a few seconds.

### `phrases`

Short melodic fragments triggered on session state changes.

| Key                    | Type  | Unit     | Range       | Description                                                                      |
|------------------------|-------|----------|-------------|----------------------------------------------------------------------------------|
| `completion_octaves`   | int   | octaves  | [1, 8]      | Upward transposition applied to each note of the completion phrase.              |
| `completion_duration`  | float | seconds  | (0, 60]     | Total duration of each note in the completion phrase.                            |
| `completion_gain`      | float | linear   | [0, 1]      | Amplitude of completion phrase notes. `NaN` is rejected.                         |
| `failure_interval`     | int   | semitones | < 0        | Transposition applied to the failure phrase. Must be negative: `PRD.md` §9 requires the failure cadence to descend. A value of 0 or positive is an error. |
| `failure_duration`     | float | seconds  | (0, 60]     | Total duration of each note in the failure phrase.                               |
| `failure_gain`         | float | linear   | [0, 1]      | Amplitude of failure phrase notes. `NaN` is rejected.                           |
| `cancelled_sounds`     | bool  | —        | —           | Whether cancellation triggers an audible phrase. Defaults to `false`.            |
| `cancelled_duration`   | float | seconds  | [0, 60]     | Duration of the cancellation note. Must be positive when `cancelled_sounds` is on. |
| `cancelled_gain`       | float | linear   | [0, 1]      | Amplitude of the cancellation note. `NaN` is rejected.                          |
| `attack`               | float | seconds  | [0, 60]     | Per-note envelope attack for phrase notes. 0 means instantaneous onset.         |
| `decay`                | float | seconds  | [0, 60]     | Per-note envelope decay for phrase notes.                                        |

**Why every duration has an upper bound.** These floats are multiplied by
`time.Second` to become a `time.Duration`, and `+Inf` or `1e308` seconds
overflows that conversion into a meaningless value. Sixty seconds is far past any
musically sensible phrase, so the bound rejects the mistake without constraining
real themes. The same reasoning applies to `drone.attack` and `drone.release`.

**`completion_octaves` note.** `PRD.md` §8 prose says "one or two octaves higher"; the worked example in §8 shows three octaves. `minimal` ships 1, the low end of the prose. Two was right when the drone sat at octave 2, but `music.octave` now defaults to 3 and harmonies sound up to two octaves above that, so a completing voice at C5 would chime at C7 — around 2 kHz at `completion_gain: 0.7`, which is piercing rather than informative. One octave keeps the whole set inside the register the drone already occupies. The daemon's own `DefaultPhraseSpec` still says 2; it has no production caller, and a theme always supplies the real value.

**`cancelled_sounds` rationale.** Cancellation is silent by default. Inventing a third audible cadence risks users conflating cancellation with failure. The flag exists as a seam for future themes that want the distinction.

## Resolution order

`Load(name)` resolves themes in this order:

1. `$HUM_HOME/themes/<name>.yaml` (where `HUM_HOME` defaults to `~/.hum`).
2. The built-in theme embedded in the binary.

If the user-supplied file exists but is malformed or fails validation, `Load` returns an error. It does **not** fall back to the embedded copy. A silent fallback would leave users editing a file that has no effect, making theme customisation impossible to debug.

To shadow a built-in theme without rebuilding:

```sh
mkdir -p ~/.hum/themes
cp /path/to/reference/minimal.yaml ~/.hum/themes/minimal.yaml
# edit ~/.hum/themes/minimal.yaml
```

## Canonical file location

`PRD.md` §18 shows a top-level `themes/` directory. The theme files are embedded inside the package at `internal/theme/themes/` instead, for two reasons:

1. `go:embed` patterns may not escape their package directory. A top-level `themes/` directory would require a separate embedding shim or a `//go:embed` directive in a different package, adding indirection.
2. A single canonical location eliminates drift. Two copies — one embedded, one at the repo root — would diverge whenever a theme is edited and the developer forgets to synchronise them.

## Waveforms

Two are accepted, `sine` and `strings`. Any other value is a validation error
rather than a silent fallback, because accepting an unsupported waveform and
rendering it as sine would silently ignore a user's intent.

`sine` is `PRD.md` §20's MVP synthesiser and is what `minimal` ships. Its output
is unchanged by the arrival of `strings`: the sine path in `Osc.Mix` still runs
the same arithmetic in the same order, so a `minimal` drone is sample-identical
to the one that shipped before.

`strings` is the warm-sound path `PRD.md` §291 asks Orchestra for. It is
selected per theme, not per voice, and phrase notes stay on the sine path
regardless, so a completion chime keeps its own character against a string pad.

## Validation

`Validate()` is called by `Load()` on every theme, embedded or user-supplied. Checks applied:

- `name` non-empty.
- `waveform` is `"sine"` or `"strings"`.
- `drone.attack` and `drone.release` are strictly positive (prevents click-on-start and inaudible release).
- `drone.gain`, `drone.harmonic`, `phrases.completion_gain`, `phrases.failure_gain` are in [0, 1] using `!(v >= 0 && v <= 1)` so that `NaN` always fails (ordinary `>` and `<` comparisons return false for NaN, which would let NaN pass a range check silently).
- `drone.tremolo_hz` is non-negative.
- `phrases.completion_octaves` is in [1, 8].
- `phrases.completion_duration` and `phrases.failure_duration` are strictly positive.
- `phrases.failure_interval` is strictly negative.
- `phrases.attack` and `phrases.decay` are non-negative.
- With `waveform: sine`, every `strings` key is unset, `NaN` included, since `NaN != 0`.
- With `waveform: strings`, `drone.harmonic` is 0, `drone.partials` is in [2, 32], `drone.cutoff_hz` is in (0, 20000], `drone.brightness_octaves` is in [0, 6], `drone.ensemble_voices` is in [1, 4], `drone.ensemble_cents` is in [0, 100] and `drone.ensemble_drift_hz` is in [0, 2], all with the `NaN`-rejecting idiom above.

## Theme names are not paths

`Load` rejects a name that is empty, `.`, `..`, or that contains a path
separator, with `ErrInvalidName`. The name arrives from `hum theme use <name>`
over the socket and is joined onto `$HUM_HOME/themes/`, so without that guard a
client could walk the daemon out of its theme directory and make it read and
parse an arbitrary `.yaml` file on the machine. Rejecting the name is cheaper
than reasoning about what the parser would do with the result.

Both the embedded and the user file are decoded with `KnownFields(true)`. The
shipped theme is held to the same standard as a user's: a key in
`internal/theme/themes/minimal.yaml` that no longer matches the struct is a bug
we want the test suite to catch, not a value silently ignored at runtime.
