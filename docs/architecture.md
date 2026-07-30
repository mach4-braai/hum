# Architecture

A contributor's map of `humd`: which goroutine owns which state, how a
session event travels from the wire to a speaker, and what breaks if you
violate the ownership rules.

The per-package documents this file links to carry the detailed reasoning;
this file is the overview that ties them together.

---

## Process split

Two binaries, one durable:

- **`humd`** is the long-lived daemon. It holds the audio device, the
  session registry, and the musical context. It binds the Unix socket and
  serves indefinitely.
- **`hum`** is a thin client. It dials the socket, writes one request,
  reads one response, and closes. It retains nothing between invocations.

The daemon must own the audio device because audio hardware is exclusive:
only one process can hold the device open. A per-command process would
acquire the device, emit a fraction of a drone, and release it — producing
a click at best, silence at worst, and never a sustained soundscape. The
daemon holds the device for its entire lifetime and hands state to the
audio thread continuously.

---

## The rendering chain

```
hum (client)
    │ Unix socket (JSON)
    ▼
transport.Listener  ←── one goroutine per connection
    │ calls channel (call struct)
    ▼
serveEvents goroutine
    │
    ├─▶ session.Registry.Apply
    │       │ Change
    │       ▼
    │   harmony.Engine.Apply
    │       │ State, []Phrase
    │       ▼
    │   renderer.Renderer.Update   (sustained voices)
    │   renderer.Renderer.Trigger  (one-shot phrases)
    │
    ▼
internal/audio  (audio renderer only)
    │ io.Reader (float32le stereo)
    ▼
oto.Player → speakers
```

The full event path, as `docs/daemon.md` summarises it:

```
request → registry.Apply → harmony.Apply → renderer.Update → renderer.Trigger
```

**Core never produces sound directly.** The harmony engine emits an
abstract `harmony.State` — a set of pitches and expression values — and
the renderer interprets that state into whatever output medium it
supports. `internal/renderer` defines the seam; `internal/audio` is one
implementation behind it.

The renderer is an interface (`renderer.Renderer`) so that future
backends — MIDI, OSC, Hue, haptics — can replace or supplement the audio
engine without touching the daemon's event loop (see PRD §19 for the full
list). The MVP ships two renderers: `audio` (the sine-wave engine in
`internal/audio`) and `nop` (used when `--no-audio` is set or no device
is present). The `nop` renderer ships in the production package, not as
a test helper, because the daemon itself uses it at runtime. See
`docs/renderer.md` for the interface, the registry, and the rules around
optional interfaces (`Themeable`, `Sampled`).

---

## Concurrency contract

This is the load-bearing section. Get this wrong and the race detector
will not always catch it.

### The event goroutine owns the engine

`serve` starts a single goroutine that runs `d.serveEvents`:

```go
go d.serveEvents(events)
```

`serveEvents` is the **only** goroutine that calls `d.registry.Apply`,
`d.engine.Apply`, `d.render.Update`, and `d.render.Trigger`. It is the
sole writer of all musical state. `harmony.Engine` carries no internal
mutex; `Engine.Apply` is not safe for concurrent use. Nothing locks the
musical state because nothing else touches it.

### How a connection goroutine reaches the event goroutine

`transport.Listener.Serve` spawns one goroutine per accepted connection.
That goroutine calls `d.handle` for each request it reads. `handle` does
not touch the engine or the registry directly. Instead it creates a
per-request reply channel and sends a `call` struct onto `d.calls`:

```go
type call struct {
    request protocol.Request
    reply   chan protocol.Response
}
```

`serveEvents` receives from `d.calls`, dispatches the call, and sends the
response back on `c.reply`. The connection goroutine blocks on `c.reply`
until the event goroutine answers. There is exactly one reader of
`d.calls` and exactly one writer per in-flight request, so no additional
lock is required.

If `d.stopped` is closed (the event goroutine has exited), `handle`
returns `"daemon is shutting down"` rather than blocking forever on a
channel nobody reads.

### Update arrives from exactly one goroutine

`renderer.Renderer.Update` is called only from the event goroutine, never
from a connection goroutine or a ticker. A renderer may hand the received
`harmony.State` to an audio callback thread for mixing — the `audio`
renderer does exactly this — but that internal handoff is the renderer's
own concern. The interface contract is: the event goroutine calls
`Update`, and the renderer serialises access to its own internal state
however it needs to.

### The mixer lock is never held across a callback

`internal/audio.Mixer` protects `sources` and `gain` with `m.mu`. `Read`
— which runs on the oto audio thread — holds the lock only long enough to
snapshot active sources and read the master gain, then releases it before
the inner mixing loop. A daemon goroutine calling `Add`, `Remove`, or
`SetGain` can therefore never deadlock the audio thread, and the audio
thread never stalls the daemon.

See `docs/audio.md` for the zero-allocation contract that makes this
hold at audio thread priority.

### The invariant a contributor will break first

**Do not call `d.engine.Apply` or `d.registry.Apply` from anywhere except
the event goroutine.**

The first violation a new contributor is likely to make is adding a
command handler — say, a `status` shortcut — that reads the engine or
registry directly on the connection goroutine rather than routing through
`d.calls`. That produces a data race that `go test -race` may not catch
in every run because the engine access may not overlap with a concurrent
event.

The correct pattern: if a new handler needs engine or registry state, add
a `call` path through `serveEvents`. All existing commands, including
`status`, follow this pattern. The `status` snapshot, the volume
commands, and the theme commands all reach the engine and renderer through
the single event goroutine.

The allocator inside `harmony.Engine` does have its own mutex (required
for `-race` coverage), but the engine itself is not safe for concurrent
`Apply` calls; the allocator's mutex only guards the allocator's own
degree map.

---

## Connection handling

`transport.NewUnixListener` binds the socket. `Serve` accepts connections
in a loop and spawns one goroutine per connection. Each goroutine reads
newline-delimited JSON requests and writes one `protocol.Response` per
request. There is no multiplexing: request and response alternate
strictly on a single connection.

The transport layer is described in detail in `docs/transport.md`,
including the per-half deadline strategy, the grace period on shutdown,
and the stale-socket probe that prevents a second daemon from starting
over an orphaned socket.

---

## Shutdown

Graceful shutdown follows the same path whether triggered by `SIGINT`,
`SIGTERM`, or the `shutdown` command. `serve` detects any of the three
and calls `d.drain`:

1. **Stop accepting connections.** `stopAccepting` cancels the context
   passed to `listener.Serve`. In-flight requests continue draining
   against the still-running event goroutine; `Serve` waits up to
   `opts.Grace` for connection goroutines to finish.

2. **Release all voices.** `drain` calls `d.render.Update(harmony.State{})`
   — an empty state — which signals the renderer to release every
   sustained drone. The audio renderer fades each voice through its
   release envelope.

3. **Wait for the fade.** `drain` waits for `d.releaseWait`, which is:

   ```go
   func releaseWaitFor(th theme.Theme) time.Duration {
       return time.Duration(th.Drone.Release*float64(time.Second)) + shutdownMargin
   }
   ```

   `shutdownMargin` is 500 ms. The total wait is the theme's release
   duration plus that margin, giving the envelope time to reach silence
   before the device is closed. Closing the device mid-drone produces a
   click; the wait exists to prevent exactly that.

4. **Close the renderer.** `d.render.Close()` tears down the audio device.
   Voices are released strictly before the renderer closes; reversing the
   order would close the device while a fade was still owed.

5. **Close the listener.** The socket file and pidfile are unlinked by
   `listener.Close()`.

A **second signal** during the wait causes `drain` to abandon the fade,
close the renderer immediately, and return `exitInterrupted` (130). An
operator must always be able to force the issue.

---

## Startup order

`run` in `cmd/humd/main.go` initialises in this order:

1. Resolve the global configuration file and load the named theme.
2. Open the renderer (`audio` by default; `nop` if `--no-audio` is set
   or `audio.NewEngine` returns `ErrNoDevice`).
3. Construct the registry (`session.New()`) and the harmony engine
   (`harmony.NewEngine`) from the resolved root, scale, and phrase spec.
4. Bind the socket (`transport.NewUnixListener`).
5. Call `serve`, which starts `serveEvents` and `listener.Serve`.

The renderer opens before the socket is bound. A daemon that fails to
open audio never accepts connections; a daemon that opens audio but
cannot bind the socket closes the renderer cleanly before exiting.

---

## Musical context

The daemon holds one musical context (root, scale, theme) at a time.
Context adoption on `session.started` is conditional: the event goroutine
calls `adoptContext` only when no sessions are currently sounding. A
joining session therefore inherits the established context rather than
overriding it. `hum status` reports `context_owner` so which project set
the context is always visible.

This resolves the conflict between PRD §7 (single key) and §12
(per-project config); the decision is tracked in issue #49 and
documented in `docs/daemon.md` under "Musical context".

## Log volume

PRD §2 requires Hum to run continuously, so the daemon's output has to be
bounded by state rather than by traffic. At the default `info` level:

| What | Level | Rate |
|---|---|---|
| Startup and shutdown | info | 2 lines per daemon lifetime |
| Lifecycle transitions | info | 1 line per non-`updated` event |
| Context adoption | info | 1 line per **change** of root, scale or owner |
| `session.updated` | debug | 0 lines by default |
| Reaping | debug | 0 lines by default |
| A repeating renderer fault | error | at most 1 line per fault per minute |
| A repeating theme fault | warn | at most 1 line per fault per minute |
| Periodic summary | info | 1 line per 5 minutes **with activity** |

**With no active sessions the steady-state volume is zero bytes per hour.**
The summary ticker fires every five minutes but returns without logging when
the interval saw no events, no reaping, no suppressed faults, and nothing is
sounding — an idle daemon has nothing to say, and `hum status` answers "what
now" on demand. One session held open across an idle hour costs 12 summary
lines, around 1.8 KB.

A session driven through 1000 `session.updated` events logs **one** line, for
the start. `--log-level=debug` restores every one of them: the detail is
demoted, never discarded.

Repeated identical faults are coalesced by `throttle`, keyed on message plus
error text, with a one-minute window. Both the renderer errors and the
"keeping the current theme" warning go through it — a project whose config
names a theme that will not load re-triggers that warning on every
`session.started` the daemon adopts, which is unbounded without it. The first
occurrence is logged immediately; the next admitted line carries `repeats=N`
for what was suppressed in between, and the summary reports the running total.
The key map is bounded at `maxThrottleKeys` and reset when it fills, because a
daemon running for months must not accumulate error strings.

`dropped_phrases` in the summary comes from `renderer.PhraseDropper`, which
the audio renderer implements by counting what the phrase-voice cap
discarded. Phrases arriving faster than they can finish is the one kind of
loss nothing else reports.

---

## Further reading

| Topic | Document |
|---|---|
| Wire protocol and message shapes | `docs/protocol.md` |
| Daemon flags, event path, control path | `docs/daemon.md` |
| CLI commands and output | `docs/cli.md` |
| Transport, deadlines, socket lifecycle | `docs/transport.md` |
| Renderer interface and concurrency contract | `docs/renderer.md` |
| Harmony engine, allocation, phrases | `docs/harmony.md` |
| Session state machine and reaping | `docs/session.md` |
| Audio engine, mixer, oscillator | `docs/audio.md` |
| Themes and phrase specs | `docs/themes.md` |
| Configuration layers | `docs/configuration.md` |
