# Audio Package

`internal/audio` is the only package in the tree that touches a sound device.
It implements the bottom of the PRD §19 chain: Harmony Engine → Renderer → **Output**.

---

## Format

`Format` carries `SampleRate` and `Channels`. `DefaultFormat()` returns 48 000 Hz, 2-channel stereo — the format used by every `oto` call in this package. The wire encoding is `oto.FormatFloat32LE`: two little-endian `float32` values per frame, left channel first.

### One player per process

`NewEngine` creates exactly one `oto.Context` and exactly one `oto.Player` fed by a single `Mixer`.  Per-voice players were evaluated and rejected for two reasons:

1. Each `oto.Player` allocates its own device ring buffer. Twelve simultaneous drone voices would hold twelve ring buffers, inflating memory and latency.
2. The audio hardware merges independent players incoherently. Phase relationships between drone voices (which define the chord) cannot be maintained across separate players because their callbacks fire at different sample offsets. A single player fed by a summing mixer gives sample-accurate phase coherence across all voices.

### Device-absent fallback

`NewEngine` returns `ErrNoDevice` (wrapped with `fmt.Errorf("%w: …")` so `errors.Is` works) when `oto.NewContext` fails. The daemon uses this to fall back to `NopRenderer` rather than crashing. The `oto` context factory is behind a package-level variable (`newOtoContext`) following the same convention used elsewhere in the tree (`var exit = os.Exit`, `var absolute = filepath.Abs`). Tests restore the original value with `t.Cleanup` before installing any stub, and never call `t.Parallel` when the seam is staged.

---

## Mixer

`Mixer` implements `io.Reader` (float32le stereo). `oto.Player` calls `Read` continuously; everything else calls `Add`/`Remove`/`SetGain` from the daemon goroutine.

### Zero-allocation contract

`Read` **must not allocate** on every call — an allocation in the audio callback path shows up as a glitch or pop. Three decisions enforce this:

1. `scratch` holds one `[][2]float32` per bus, each preallocated to `maxScratchFrames` (4 096 frames) in `NewMixer`. If `Read` is called with more frames than `maxScratchFrames`, it processes them in batches without growing any slice.
2. `active []sourceEntry` is preallocated with `cap = maxSources` (32). Under a short lock, `Read` resets it to length zero (`active[:0]`) and copies live sources from the map. Because 32 ≥ `harmony.MaxVoices` (12) plus all simultaneous phrase notes, the append never triggers a reallocation.
3. `done []doneSource` is similarly preallocated. The ids it carries are copied from map keys, which are two-word header copies — no heap allocation.

The zero-alloc invariant is asserted by `TestMixerReadZeroAlloc` using `testing.AllocsPerRun(100, …)`.

### Thread safety

`sources` and `gain` are protected by `m.mu`. `Read` holds the lock only long enough to snapshot `active` and read `gain`, then releases it before the inner loop. This ensures that a daemon goroutine calling `Add`/`Remove`/`SetGain` can never deadlock the audio thread.

`active` and `done` are used exclusively inside `Read` and are not mutex-protected. `Read` is single-consumer (driven by one oto audio thread).

### Two buses

A source joins either `DroneBus` or `PhraseBus`, named at `Add`. The buses are normalised separately and summed immediately before the clipper.

They exist because the two kinds of sound answer different questions. A drone is *sustaining*, and what matters is that adding a thirteenth session does not shrink the other twelve. A phrase note is a *transient*, and what matters is that it arrives at the level the theme asked for. One shared divisor cannot serve both: with every source in one count, a `hum complete` took the divisor from N to N+1 and every sustaining drone dropped by `20·log10(N/(N+1))` dB for the length of the chime — 6 dB with one drone. The chime the user was meant to hear arrived on top of an audible dip in everything else.

`Mixer` does not parse id prefixes to work out which is which. `AudioRenderer` already knows, and says so at the call.

`TestCompletionChimeDoesNotDuckOneDrone` and `TestFailureCadenceDoesNotDuckTwoDrones` render the drone alone, the chime alone, and both, then check the drone's contribution to the mix is the same either way. `math.Atanh` recovers the pre-clipper sum, so the two contributions can be separated exactly. `TestChimeLevelIsIndependentOfHowManyDronesSound` is the same measurement from the other side.

### Normalisation

Normalisation lives in **the Mixer**, not in the oscillator. The oscillator does not know how many peers are active. After snapshotting active sources, `Read` counts them per bus and sets one target per bus:

```
droneNorm  = masterGain / max(1, droneCount)
phraseNorm = masterGain / sqrt(max(1, phraseCount))
```

The drone curve holds the *summed* level of twelve coherent drones equal to one, which is what `1/N` corrects for.

The phrase curve already assumes incoherence, because the measurement forced it. Sixteen notes is `maxPhraseVoices`, the cap a burst of completions hits, and at a fixed per-note gain those sixteen drove the clipper to a pre-`tanh` peak of 3.14 at `volume: 0.6` and 10 dB of gain reduction: not a chime, a crunch. Dividing by `√16` brings that to 1.46 and 4.2 dB. `TestPhraseVoicesSumIncoherentlyRatherThanLinearly` holds the curve.

#### The chime got louder, so the theme gains came down 2.92 dB

A single chime divides by one, so the phrase bus scales it by the master gain alone. The shared divisor scaled it by `masterGain / (N+1)`. Against one drone that is a factor of two: **the chime term is +6.02 dB**, and +22.3 dB against twelve.

That is not a neutral refactor, so `completion_gain` and `failure_gain` moved from 0.7 and 0.35 to 0.5 and 0.25. Measured as the audible lift of the whole mix — RMS over the chime's 250 ms against the same window of undisturbed drones, `volume: 0.6`:

| drones | shared divisor, gain 0.7 | separate buses, gain 0.7 | separate buses, gain 0.5 |
|---|---|---|---|
| 1 | -0.03 dB | +4.37 dB | +2.79 dB |
| 2 | +0.12 dB | +6.67 dB | +4.62 dB |
| 12 | +0.08 dB | +13.68 dB | +11.02 dB |

The first column is the bug in one number: a chime raised the total mix level by nothing, because the duck removed as much energy as the chime added. That is an energy measurement and not an audibility one — `completion_octaves: 1` puts the chime an octave above the drone it belongs to, so it stays spectrally distinct and a listener separates it regardless. What the column shows is that the chime bought no headroom in the mix, not that nobody could hear it.

Read the columns as combined-mix RMS lift, which is not the chime's level against the drone bed. Measured directly, bus against bus, the chime sits **-0.02 dB against the bed** at the shipped gain and +2.9 dB at the old one. Two equal uncorrelated sources give a +3.01 dB lift, which is exactly why a +3.00 dB figure reads as "3 dB above the drones" and means the opposite. The gains themselves moved by a factor of 5/7, or -2.92 dB, not by half; what is halved is failure against completion, in both the old pair and the new one.

The last column is still voice-count dependent, because the drone bus keeps `1/N` at this point. Flattening it is the drone curve's job, not the phrase gain's, and the gains were chosen against the flat figure the next change produces.

#### The divisor counts sounding sources, not scheduled ones

`Trigger` adds every note of a phrase to the mixer at once, and a note with a non-zero `Offset` then sits there emitting silence until its sample countdown elapses. Counting it while it waits would divide the notes that *are* sounding by a source contributing nothing.

The failure cadence is exactly that shape: `internal/harmony` emits two notes, the second at `Offset: FailureDuration`. Both enter the mixer at trigger time, so a naive count made the divisor 2 while only the first note sounded, and the cadence came out **2.45 dB lopsided** — first note quiet, second note full. A test built from hand-written notes at offset 0 cannot see this, because then the count of 2 is correct.

`Read` therefore counts a source toward its bus's divisor only from the frame it sounds. `Delayed.FramesUntilOnset` reports how many frames a source is still waiting; `phraseSource` returns its remaining offset. Sources already sounding are counted before the batch, and an onset landing inside the batch is collected, sorted, and applied in the sample loop at that exact frame, so the divisor never depends on where a buffer boundary happens to fall. A separate tally of sources *present* on the bus decides whether the bus is cleared and summed, because a waiting source must keep receiving `Mix` calls — that is what advances its countdown.

Three tests divide that claim up, and the split matters because two of them cannot see the onset logic at all.

`TestADelayedNoteSoundsTheSameAtAnyBlockSize` renders one offset note and requires the level identical to 1e-9 across nine block sizes from 1 to 8 192 frames. It proves the note's *duration* lands on the right sample, and nothing about the divisor: a lone source divides by one whether or not it is counted while waiting.

`TestTheTwoFailureNotesSoundAtTheSameLevel` renders the real cadence at the same sizes within 0.5 dB. Measured over a note's whole 300 ms a late divisor averages away to under half a dB, so this one does not catch it either.

`TestAnOnsetInsideABufferTakesTheDivisorAtThatFrame` is the one that does. The cadence's second note begins at frame 14 400, so the sweep lands its onset 442, 64, 2 115, 2 112, 2 109, 4 400 and 6 208 frames into a buffer, the last two inside the *second* scratch batch since `maxScratchFrames` is 4 096. Measuring only the 1 500 frames from the onset, where the error lives rather than where it dilutes, a count taken before `Mix` reads 0.52 to 0.74 dB hot at every one of those sizes.

An empty phrase bus costs nothing: `Read` skips clearing its scratch buffer and drops the multiply-add from the sample loop, and snaps the phrase ramp to its target rather than stepping it. Nothing can click through a bus carrying no signal. Measured against a single-bus `Read`, `BenchmarkMixerRead/minimal` at twelve voices is unchanged inside the run-to-run spread.

### The output gain ramp

`norm` is a product of two things that both change abruptly: the master gain, which a user moves with `hum volume` or `hum mute`, and the voice count, which moves when a session starts or ends. Applying either change between one sample and the next steps the waveform, and a step is a click.

`normRamp` therefore sits between `norm` and the multiply. `SetGain` and the voice count set a *target*; `Read` calls `normRamp.step` once per frame, and each step moves `current` toward `target` by a one-pole with a `rampTimeConstant` of 40 ms. Every gain change in the package goes through it, so there is exactly one ramp shape and one place to change it.

Two consequences are load-bearing.

**Nothing outside the mixer ramps.** `AudioRenderer.SetMuted` is `SetGain(0)` and unmute is `SetGain(volume)`, so mute and unmute are mirror images: at sample *n* they sum to the configured volume. An earlier version faded mute in a goroutine that slept between twenty `SetGain` calls, which produced twenty 2.5 ms plateaus rather than a ramp, left unmute stepping, and made the trajectory depend on the Go scheduler rather than the sample clock. `TestMuteAndUnmuteAreMirrorImages` and `TestGainRampHasNoPlateausInEitherDirection` hold both properties.

**A one-pole never arrives, so `step` snaps.** When `|target - current|` falls below `rampSettle` (1e-9) the ramp assigns the target outright. Without it, muting leaves a gain of order 1e-16 multiplying a live signal forever: not audible, but not silence either, and a denormal tail in the hot loop. `TestGainRampSettlesExactlyOnZero` asserts the tail is bit-exact zero.

`Mixer.Gain` returns the target, not `current`. It answers "what is the volume set to", which is what status and the config file mean by volume; the instantaneous value is an artefact of the fade and is observable from the output samples, which is how the tests read it.

### Soft-clipping

After normalisation, each sample is passed through `math.Tanh`. This is a smooth, differentiable limiter: at input 1.0 it outputs ≈ 0.76; at input 3.0 it outputs ≈ 0.995. It can never produce a value outside (−1, 1), preventing hard clipping regardless of transient overdrive from simultaneous phrase triggers. `tanh` was chosen over a hard limiter because it introduces no discontinuity and over a lookahead limiter because it requires no look-ahead buffer (which would add latency and an allocation).

---

## Oscillator

`Osc` is a stereo oscillator with an ADSR envelope that implements `Source`. It
carries two waveform paths, selected by `SetTone`: a single sine plus an optional
second harmonic, and a band-limited partial stack played by a detuned ensemble
through a low-pass filter. `ToneOf` maps a theme onto the choice — `waveform:
sine` yields the zero `Tone` and keeps the sine path, `waveform: strings` fills
it in. Phrase notes always take the sine path: `newPhraseSource` never calls
`SetTone`, so a completion chime stays a pure tone that reads as a separate
gesture against a string pad.

### Envelope

The envelope has three active phases: attack, sustain, and release.

| Phase   | Behaviour |
|---------|-----------|
| Attack  | `curGain` increments linearly by `initGain / attackSamples` each sample until it reaches `initGain`. |
| Sustain | `curGain` tracks `tgtGain` (set by `SetGain`) via a 1st-order IIR with `gainSmoothAlpha = 0.005` (~200-sample time constant at 48 kHz). |
| Release | `curGain` decrements linearly by `peakGain / releaseSamples` each sample. `peakGain` is captured at the moment `Release()` is called, so an interrupted attack fades from wherever the gain happened to be. |

Default values come from `minimal.yaml`: **attack 2.5 s**, **release 3.0 s** (`drone.attack` / `drone.release`). The `Envelope` struct is populated by the renderer from `theme.DroneSpec`.

`Mix` returns `done = true` only after the release phase has fully elapsed (all release samples consumed or `curGain` reaches zero).

**`Mix` adds; it never assigns.** Every source on a bus mixes into the same scratch buffer, and the `Mixer` clears that buffer once at the top of each batch. A source that finishes therefore stops adding and returns — it must not zero the remainder. An earlier version did zero it, on the reasoning that the mixer could then remove the source cleanly, and that was wrong twice: the clearing is already the mixer's job, and assigning over shared scratch erased whatever peers had mixed in first. Because `Read` snapshots sources by ranging a map, "first" is randomised per process, so the bug showed up as output that changed between runs of the same input — measured at up to 1.2 dB on a two-note phrase. `TestOscReleaseYieldsDone` and `TestOscMixAlreadyDone` prefill the buffer with a peer's samples and require them to survive.

### Click avoidance

Frequency is interpolated per-sample toward `tgtFreqL` / `tgtFreqR` using a 1st-order IIR with `freqSmoothAlpha = 0.01` (~100-sample / 2 ms time constant at 48 kHz). A `SetFreq` or `SetExpression` call changes the target; the running phase accumulator is never reset, so the sine wave is always continuous. The same smoothing applies across `Read` buffer boundaries.

Two phase-continuity thresholds are asserted, because the two waveforms have
different legitimate slew.

**Sine: 0.10** (absolute amplitude). For a 440 Hz sine at gain 0.8 the largest
per-sample step through the full pipeline measures **0.0326**: the phase
increment is 2π × 440 / 48000 ≈ 0.0576, and the amplitude reaching the buffer is
`curGain × invSqrt2` ≈ 0.566, so the derivative of the sine is ≈ 0.566 × 0.0576.
That leaves roughly 3× headroom for the second harmonic and tremolo.
`TestOscPhaseContinuity` and `TestOscPhaseContinuityMidBufferFreqChange` assert
it.

**Strings: 0.35.** A partial stack has a flyback, and the low-pass is what bounds
how fast it travels: after two cascaded one-poles at cutoff `fc`, the full
peak-to-peak excursion takes about half a cycle of `fc`, so the largest step is
≈ `4 × peak × fc / sampleRate`. For the `orchestra` values at gain 0.8 the peak
reaching the buffer is `curGain × invSqrt2 × √voices × 1.31` ≈ 1.285. The 1.31 is
the crest factor of an RMS-normalised 12-partial stack, and the `√voices` comes
from the ensemble normalisation below. `fc` at `Intensity = 1` is
1500 × 2^0.8 ≈ 2610 Hz. That gives 4 × 1.285 × 2610 / 48000 = **0.279**, which
is what `TestStringsPhaseContinuityAcrossBuffers` measures over eight seconds at
every drone pitch. Full-depth tremolo scales it by 1.1 to 0.307, and 0.35 leaves
the rest as headroom. A real click is a step of order the peak itself, 1.285, so
the threshold still catches one with 3.7× to spare.

### Partial stack and wavetable

`waveform: strings` plays `Σ sin(nθ)/n` for `n = 1 … drone.partials`: odd and
even partials at 1/n, the spectrum of a sawtooth, which is the raw material a
low-pass turns into a string or pad. Computing that sum per sample would cost
`partials × channels × ensemble_voices` sine calls per frame, 72 at the
`orchestra` values, so `internal/audio/wavetable.go` precomputes it.

- One table per octave band: `tableBands = 11` bands of `tableSize = 2048`
  samples, starting at `tableBaseHz = 16` Hz, so the set spans 16 Hz to 32 kHz.
- Band *k* keeps only the partials that stay below `tableBandLimit = 0.45` of the
  sample rate at the **top** of the band, so nothing in it can alias. A stack
  therefore thins as pitch rises, which is also what a real instrument does.
- The band is chosen from `math.Frexp` rather than `math.Log2`, because the
  exponent of a float64 already is the octave. Selection uses
  `max(curFreq, tgtFreq)` so a glide in either direction lands on the safer,
  thinner table.
- Every band is scaled by one factor, computed so band 0 carries the RMS of a
  unit sine (`invSqrt2`). That is what keeps `drone.gain` meaning the same thing
  across both waveforms; see § Level parity.
- Tables are built once per `(sampleRate, partials)` pair and cached at package
  level under a mutex. `NewOsc` never builds one; `SetTone` does, off the audio
  callback. `Mix` only interpolates, so the zero-allocation contract holds, and
  `TestStringsMixDoesNotAllocate` asserts it separately from the sine case.
- Each table holds `tableSize + 1` samples, the last a copy of the first. Linear
  interpolation can then read `samples[i+1]` with no wrap branch.

### Ensemble

`drone.ensemble_voices` copies of the stack sound per channel, spread across
`±ensemble_cents / 2` and each drifting by up to `driftDepth = 0.5` of that
window under its own LFO. The LFO rates differ by `driftSpread = 0.37` per copy,
so no two copies ever return to the same relationship. One player and one
detuned pair sound like one instrument; three copies moving independently are
what makes a section.

Two costs are avoided deliberately:

- The drift LFOs and the per-copy target frequencies are recomputed every
  `toneControlRate = 64` samples, not every sample. The per-sample frequency IIR
  (`freqSmoothAlpha`) smooths the 1.3 ms staircase away, so nothing is audible.
- Copies start phase-**coherent**. Spreading their initial phases evenly is the
  obvious-looking choice and it is wrong. Three copies of a stack at 0°, 120° and
  240° cancel every partial whose index is not a multiple of three, which
  measured 6 dB down and thin. The detune pulls them apart on its own within a
  beat period.

### Level parity

Detuned copies are summed and divided by `√voices`, not by `voices`. The
division by `voices` is right only while the copies are coherent; a detuned
ensemble spends most of its time incoherent, where it costs a further factor of
`√voices`, 4.8 dB at three copies. `√voices` normalises for the incoherent case
and lets the coherent moments swell above it, which is the chorusing the
ensemble exists to produce.

Measured at `drone.gain = 0.5`, `Intensity = 1`, over eight seconds, `orchestra`
lands +0.7 dB against `minimal` at C3, −0.7 dB at C4 and −1.2 dB at A4.
`TestStringsHoldsTheLevelOfTheSineItReplaces` holds the drone register to 3 dB.
Above the register the low-pass takes more of the stack away: A5 measures
−2.9 dB, and that is the intended trade rather than a defect.

### Expression mapping

`SetExpression(harmony.Expression, theme.DroneSpec)` maps three expression axes:

| Expression field | Mapping |
|-----------------|---------|
| `Intensity` | On the sine path, blends in a quiet second harmonic: `harmonic = Intensity × DroneSpec.Harmonic`, where the harmonic is `sin(2θ)` on the running fundamental phase, so no separate accumulator is needed. On the strings path it opens the low-pass instead: `fc = DroneSpec.CutoffHz × 2^(Intensity × DroneSpec.Brightness)`, clamped to `filterCeiling = 0.45` of the sample rate. `orchestra` moves 1500 Hz to 2610 Hz across the axis. Both mappings are the same gesture: a busier session gets a brighter voice. |
| `Tremolo` | Scales amplitude modulation depth: `tremoloScale = 1 + Tremolo × 0.10 × sin(2π × TremoloHz × t)`. At `Expression.Tremolo = 1.0` and `DroneSpec.TremoloHz = 5.0 Hz` (minimal theme default), the amplitude oscillates ±10%. |
| `Width` | Expands stereo spread via inter-channel detune: `freqL = baseFreq × 2^(Width × DetuneCents / 2400)`, `freqR = baseFreq × 2^(−Width × DetuneCents / 2400)`. On the strings path the same offset is added to every ensemble copy's cents. Both channels carry equal amplitude (`curGain × invSqrt2`) so L² + R² is constant regardless of `Width` — constant-power stereo. |

### Cost

`BenchmarkOscMix` and `BenchmarkMixerRead`, 1024-frame buffers, `Intensity`,
`Tremolo` and `Width` all at 1, zero allocations in every case. Medians of five
`-benchtime 2s` runs on an idle Apple M4. **Before** is the commit that added
neither waveform path, measured from a clean clone with the same benchmark file;
a single run varies by up to 10% on this machine, which is why the table is
medians and why the ratio is the finding rather than the absolute numbers.

| Benchmark | before, ns/op | after, ns/op | per second of audio | one core |
|-----------|---------------|--------------|---------------------|----------|
| `OscMix/minimal` | 43 836 | 43 478 | 2.04 ms | 0.20% |
| `OscMix/orchestra` | n/a | 32 170 | 1.51 ms | 0.15% |
| `MixerRead/minimal`, 12 voices | 602 617 | 611 321 | 28.7 ms | 2.87% |
| `MixerRead/orchestra`, 12 voices | n/a | 386 309 | 18.1 ms | 1.81% |

Two findings. The sine path did not move: the `o.table != nil` branch added to
`Mix` costs less than the run-to-run spread, so `minimal` is as cheap as it was.
And the stack is **cheaper** than the sine it replaces, by 36% at twelve voices,
because the sine path calls `math.Sin` four times per frame (two channels,
fundamental and second harmonic) while the strings path calls it once for the
tremolo and otherwise interpolates six table entries, three copies per channel.

### Voice-count normalisation placement

Normalisation by active source count lives in the **Mixer** (see § Normalisation above). The `Osc` knows only its own gain; the Mixer counts sources per bus and applies each bus's divisor after summing that bus.

---

## Renderer

`AudioRenderer` bridges `harmony.State` to the `Mixer`. It implements `renderer.Renderer` and registers itself under the name `"audio"` from an `init` function, following the `database/sql` driver convention used throughout the renderer registry.

### Why diffing lives here, not in the daemon

The daemon calls `Update(harmony.State)` on every state change — including transitions where no session changed pitch, gain, or expression. If the daemon were responsible for diffing (tracking which sessions are new, surviving, or gone), every renderer implementation would need to replicate the same logic. Instead, `AudioRenderer.Update` performs the diff itself: it maintains a map of active oscillators keyed by session ID and reconciles it against the incoming state on each call. The renderer interface is therefore idempotent by construction — two identical `Update` calls are a no-op at the audio level, with no new oscillators created and no gain discontinuity introduced.

### New, surviving, and vanished voices

On each `Update`, three cases are resolved:

- **New voice** — session ID absent from `active`: a new `Osc` is created via `NewOsc`, added to the mixer, and recorded in `active`.
- **Surviving voice** — session ID present in `active` with a different `VoiceState`: `SetFreq`, `SetGain`, and `SetExpression` are called on the existing `Osc`. Because the Osc's phase accumulator is never reset, the transition is click-free.
- **Vanished voice** — session ID present in `active` but absent from incoming state: `osc.Release()` is called, the entry is removed from `active`, and the Osc stays in the mixer.

### Why a vanished voice is released rather than deleted

Calling `mixer.Remove` on a sounding voice would truncate its output mid-sample, producing a hard click. `osc.Release()` instead starts a linear release ramp whose duration comes from `theme.DroneSpec.Release`. The Osc continues to feed samples into the mixer until the release fully completes, at which point `Mix` returns `done = true` and the mixer removes it automatically. This is the mechanism PRD §1 requires — Hum exists to avoid jarring transients.

### Mutex contract

`AudioRenderer` holds `mu` to protect the `active` map and the `volume` and `muted` fields. The constraint from `docs/renderer.md` is maintained: `mu` is never held while calling `mixer.Read` (the audio callback path). `mixer.Add`, `mixer.Remove`, and `mixer.SetGain` acquire only the mixer's own internal lock and do not call back into the renderer, so holding `mu` across them is safe.

### Mute and volume

`SetMuted` and `SetVolume` both write the field they own under `mu` and then call `applyGain`, which is `SetGain(0)` when muted and `SetGain(volume)` otherwise. Neither knows how a gain change is faded; the mixer's ramp is the only mechanism, described under § The output gain ramp.

Storing `volume` separately from the mixer gain is what lets `hum volume 0.4` take effect while muted: the mixer stays at zero and the new value is what unmute restores. `renderer.Options.Volume` is never defaulted, so a configured `volume: 0` starts silent rather than being replaced with a theme gain.

### `ErrNoDevice` unwrapping

The `init`-registered constructor calls `NewEngine`. When `NewEngine` returns `ErrNoDevice`, the constructor returns that error as-is (wrapped with `%w` in format.go, so `errors.Is(err, ErrNoDevice)` matches). The daemon uses `errors.Is` to detect a missing device and fall back to `NopRenderer` without exiting — the audio backend is optional when the daemon runs headless.

---

## Phrases

Phrase playback is implemented by `phraseSource`, a `Source` wrapping an `Osc` with a sample-countdown-based offset and duration gate. Each `Note` in a `harmony.Phrase` becomes one `phraseSource` on the mixer's `PhraseBus`.

### Why sample-accurate scheduling instead of wall-clock timers

A `time.Sleep` goroutine per note would schedule notes relative to the wall clock. The audio device runs on its own clock, driven by buffer-drain callbacks. Under load, the wall clock and the audio clock diverge: a note sleeping for 250 ms wakes anywhere from 245 ms to 260 ms later, depending on OS scheduling, while the audio clock has advanced exactly 250 ms × 48 000 Hz = 12 000 samples. The result is jitter audible as timing inconsistency between consecutive phrases.

`phraseSource.Mix` instead maintains an `offsetSamples` counter. Each call to `Mix` decrements the counter by the number of frames in the buffer before passing any remainder to the inner `Osc`. Because `Mix` is driven by the same audio clock that consumes samples, the offset is exact to the sample — zero jitter regardless of wall-clock load.

The note's **end** is exact for the same reason, and used not to be. `Mix` once subtracted the whole buffer length from `durationSamples` and released when the result went non-positive, which rounded a note's length up to the next buffer boundary: a 300 ms note released as much as 85 ms early at a 4 096-frame buffer, so its level depended on the audio device's block size. `Mix` now splits the buffer at `durationSamples`, mixes the sustained part, calls `Release`, and mixes the tail. `TestADelayedNoteSoundsTheSameAtAnyBlockSize` renders one offset note at nine block sizes from 1 to 8 192 frames, including sizes that straddle `maxScratchFrames`, and requires the measured level to be identical to 1e-9.

### Percussive envelope

Phrase notes use the same `Osc` and `Envelope` types as drone voices, but with values from `theme.PhrasesSpec.Attack` and `.Decay` rather than `DroneSpec`. Typical themes set a much shorter attack (5 ms vs. 2.5 s) and a decaying release rather than a sustained hold, giving a percussive character. The attack-decay profile is still click-free: the attack ramp prevents a step discontinuity at note start, and `osc.Release()` is called after the note's `Duration` elapses so the decay ramp prevents a step at note end.

### Phrase voice cap and drop-oldest policy

Concurrent phrase voices are capped at 16 (`maxPhraseVoices`). `AudioRenderer` maintains `phraseIDs`, a FIFO of mixer source IDs for active phrase notes. Before adding a new note, the list is compacted by evicting IDs that the mixer has already auto-removed (done sources). If the list is still at capacity after compaction, `mixer.Remove` is called on the oldest ID. The source is hard-removed rather than released because:

1. Capacity enforcement exists to bound memory under a burst of completions. Releasing (instead of removing) would leave all 16+ sources playing their decay tails simultaneously, defeating the cap.
2. Phrase voices are percussive and short. Cutting an old tail to make room for a new note is the less harmful trade-off: the new completion is what the user initiated.

### Self-removal

When the duration countdown reaches zero, `phraseSource.Mix` calls `osc.Release()` and sets `released = true`. On subsequent calls, the Osc completes its release phase and returns `done = true`, which causes the Mixer to remove the source automatically. `phraseSource` never calls `mixer.Remove` on itself — the mixer's own done-detection loop handles it. This keeps the phrase-source logic stateless with respect to the mixer and avoids any ID-tracking inside `phraseSource`.
