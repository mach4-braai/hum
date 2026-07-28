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

1. `scratch [][2]float32` is preallocated to `maxScratchFrames` (4 096 frames) in `NewMixer`. If `Read` is called with more frames than `maxScratchFrames`, it processes them in batches without growing any slice.
2. `active []sourceEntry` is preallocated with `cap = maxSources` (32). Under a short lock, `Read` resets it to length zero (`active[:0]`) and copies live sources from the map. Because 32 ≥ `harmony.MaxVoices` (12) plus all simultaneous phrase notes, the append never triggers a reallocation.
3. `doneIDs []string` is similarly preallocated. Strings copied from map keys are two-word header copies — no heap allocation.

The zero-alloc invariant is asserted by `TestMixerReadZeroAlloc` using `testing.AllocsPerRun(100, …)`.

### Thread safety

`sources` and `gain` are protected by `m.mu`. `Read` holds the lock only long enough to snapshot `active` and read `gain`, then releases it before the inner loop. This ensures that a daemon goroutine calling `Add`/`Remove`/`SetGain` can never deadlock the audio thread.

`active` and `doneIDs` are used exclusively inside `Read` and are not mutex-protected. `Read` is single-consumer (driven by one oto audio thread).

### Voice-count normalisation

Normalisation by active voice count lives in **the Mixer**, not in the oscillator. The oscillator does not know how many peers are active. After snapshotting active sources, `Read` sets:

```
norm = masterGain / max(1, voiceCount)
```

Each frame of the summed scratch buffer is multiplied by `norm` before soft-clipping. Result: twelve drones at sustain gain 0.5 produce the same average output level as one drone at sustain gain 0.5.

### Soft-clipping

After normalisation, each sample is passed through `math.Tanh`. This is a smooth, differentiable limiter: at input 1.0 it outputs ≈ 0.76; at input 3.0 it outputs ≈ 0.995. It can never produce a value outside (−1, 1), preventing hard clipping regardless of transient overdrive from simultaneous phrase triggers. `tanh` was chosen over a hard limiter because it introduces no discontinuity and over a lookahead limiter because it requires no look-ahead buffer (which would add latency and an allocation).

---

## Oscillator

`Osc` is a stereo sine-wave oscillator with an ADSR envelope that implements `Source`.

### Envelope

The envelope has three active phases: attack, sustain, and release.

| Phase   | Behaviour |
|---------|-----------|
| Attack  | `curGain` increments linearly by `initGain / attackSamples` each sample until it reaches `initGain`. |
| Sustain | `curGain` tracks `tgtGain` (set by `SetGain`) via a 1st-order IIR with `gainSmoothAlpha = 0.005` (~200-sample time constant at 48 kHz). |
| Release | `curGain` decrements linearly by `peakGain / releaseSamples` each sample. `peakGain` is captured at the moment `Release()` is called, so an interrupted attack fades from wherever the gain happened to be. |

Default values come from `minimal.yaml`: **attack 2.5 s**, **release 3.0 s** (`drone.attack` / `drone.release`). The `Envelope` struct is populated by the renderer from `theme.DroneSpec`.

`Mix` returns `done = true` only after the release phase has fully elapsed (all release samples consumed or `curGain` reaches zero). The remaining frames in the buffer are zeroed before returning. This ensures the `Mixer` removes the source cleanly — it can never truncate a fade mid-sample.

### Click avoidance

Frequency is interpolated per-sample toward `tgtFreqL` / `tgtFreqR` using a 1st-order IIR with `freqSmoothAlpha = 0.01` (~100-sample / 2 ms time constant at 48 kHz). A `SetFreq` or `SetExpression` call changes the target; the running phase accumulator is never reset, so the sine wave is always continuous. The same smoothing applies across `Read` buffer boundaries.

The documented phase-continuity threshold is **0.10** (absolute amplitude). For a
440 Hz sine at gain 0.8 the largest per-sample step through the full pipeline
measures **0.0326**: the phase increment is 2π × 440 / 48000 ≈ 0.0576, and the
amplitude reaching the buffer is `curGain × invSqrt2` ≈ 0.566, so the derivative
of the sine is ≈ 0.566 × 0.0576. That leaves roughly 3× headroom for the second
harmonic and tremolo. `TestOscPhaseContinuity` and
`TestOscPhaseContinuityMidBufferFreqChange` assert the threshold.

### Expression mapping

`SetExpression(harmony.Expression, theme.DroneSpec)` maps three expression axes:

| Expression field | Mapping |
|-----------------|---------|
| `Intensity` | Blends in a quiet second harmonic: `harmonic = Intensity × DroneSpec.Harmonic`. The harmonic is `sin(2θ)` where `θ` is the running fundamental phase — no separate phase accumulator needed. |
| `Tremolo` | Scales amplitude modulation depth: `tremoloScale = 1 + Tremolo × 0.10 × sin(2π × TremoloHz × t)`. At `Expression.Tremolo = 1.0` and `DroneSpec.TremoloHz = 5.0 Hz` (minimal theme default), the amplitude oscillates ±10%. |
| `Width` | Expands stereo spread via inter-channel detune: `freqL = baseFreq × 2^(Width × DetuneCents / 2400)`, `freqR = baseFreq × 2^(−Width × DetuneCents / 2400)`. Both channels carry equal amplitude (`curGain × invSqrt2`) so L² + R² is constant regardless of `Width` — constant-power stereo. |

### Voice-count normalisation placement

Normalisation by active voice count lives in the **Mixer** (see § Mixer above). The `Osc` knows only its own gain; the Mixer applies the per-call count divisor after summing all sources.

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

`AudioRenderer` holds `mu` to protect the `active` map, the `volume`, `muted`, and `rampGen` fields. The constraint from `docs/renderer.md` is maintained: `mu` is never held while calling `mixer.Read` (the audio callback path). `mixer.Add`, `mixer.Remove`, and `mixer.SetGain` acquire only the mixer's own internal lock and do not call back into the renderer, so holding `mu` across them is safe.

### Mute ramp

`SetMuted(true)` ramps the mixer's master gain from its current value to zero over 50 ms via a background goroutine (`doRamp`). A hard cut to zero would produce a click identical to deleting a sounding voice mid-sample. `SetMuted(false)` restores the master gain to the stored `volume` immediately — no ramp needed on unmute because the gain starts at zero and any attack transient is masked by silence.

Ramp cancellation uses a generation counter (`rampGen`): each `SetMuted` call increments the counter and passes the current value to `doRamp`. Before each gain step, the goroutine reacquires `mu` and compares its captured generation against `r.rampGen`; a mismatch means a newer `SetMuted` call superseded this ramp, and the goroutine exits without applying the step. This prevents a stale ramp goroutine from overwriting a freshly restored volume.

### `ErrNoDevice` unwrapping

The `init`-registered constructor calls `NewEngine`. When `NewEngine` returns `ErrNoDevice`, the constructor returns that error as-is (wrapped with `%w` in format.go, so `errors.Is(err, ErrNoDevice)` matches). The daemon uses `errors.Is` to detect a missing device and fall back to `NopRenderer` without exiting — the audio backend is optional when the daemon runs headless.

---

## Phrases

Phrase playback is implemented by `phraseSource`, a `Source` wrapping an `Osc` with a sample-countdown-based offset and duration gate. Each `Note` in a `harmony.Phrase` becomes one `phraseSource` in the mixer.

### Why sample-accurate scheduling instead of wall-clock timers

A `time.Sleep` goroutine per note would schedule notes relative to the wall clock. The audio device runs on its own clock, driven by buffer-drain callbacks. Under load, the wall clock and the audio clock diverge: a note sleeping for 250 ms wakes anywhere from 245 ms to 260 ms later, depending on OS scheduling, while the audio clock has advanced exactly 250 ms × 48 000 Hz = 12 000 samples. The result is jitter audible as timing inconsistency between consecutive phrases.

`phraseSource.Mix` instead maintains an `offsetSamples` counter. Each call to `Mix` decrements the counter by the number of frames in the buffer before passing any remainder to the inner `Osc`. Because `Mix` is driven by the same audio clock that consumes samples, the offset is exact to the sample — zero jitter regardless of wall-clock load.

### Percussive envelope

Phrase notes use the same `Osc` and `Envelope` types as drone voices, but with values from `theme.PhrasesSpec.Attack` and `.Decay` rather than `DroneSpec`. Typical themes set a much shorter attack (5 ms vs. 2.5 s) and a decaying release rather than a sustained hold, giving a percussive character. The attack-decay profile is still click-free: the attack ramp prevents a step discontinuity at note start, and `osc.Release()` is called after the note's `Duration` elapses so the decay ramp prevents a step at note end.

### Phrase voice cap and drop-oldest policy

Concurrent phrase voices are capped at 16 (`maxPhraseVoices`). `AudioRenderer` maintains `phraseIDs`, a FIFO of mixer source IDs for active phrase notes. Before adding a new note, the list is compacted by evicting IDs that the mixer has already auto-removed (done sources). If the list is still at capacity after compaction, `mixer.Remove` is called on the oldest ID. The source is hard-removed rather than released because:

1. Capacity enforcement exists to bound memory under a burst of completions. Releasing (instead of removing) would leave all 16+ sources playing their decay tails simultaneously, defeating the cap.
2. Phrase voices are percussive and short. Cutting an old tail to make room for a new note is the less harmful trade-off: the new completion is what the user initiated.

### Self-removal

When the duration countdown reaches zero, `phraseSource.Mix` calls `osc.Release()` and sets `released = true`. On subsequent calls, the Osc completes its release phase and returns `done = true`, which causes the Mixer to remove the source automatically. `phraseSource` never calls `mixer.Remove` on itself — the mixer's own done-detection loop handles it. This keeps the phrase-source logic stateless with respect to the mixer and avoids any ID-tracking inside `phraseSource`.
