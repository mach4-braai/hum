# Daemon

`humd` owns audio and session state. `hum` is a thin client over a Unix socket
and holds no state of its own.

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--config <path>` | `$HUM_HOME/config.yaml` | global configuration file |
| `--socket <path>` | `$HUM_SOCKET`, else `$HUM_HOME/humd.sock` | socket to bind |
| `--renderer <name>` | `audio` | registered renderer to open |
| `--no-audio` | off | force the `nop` renderer |
| `--log-level <level>` | `info` | `debug`, `info`, `warn` or `error` |
| `--version` | | print the version and exit |

`--config` names a *file*, not a directory, so a supervisor can point the daemon
at configuration that does not live under `$HUM_HOME`. It also becomes the global
layer for every per-session resolution, so a session's project config still
layers over the same file the daemon started with.

## One goroutine owns the music

A single event goroutine owns the session registry and the harmony engine.
Transport handlers send a request over a channel and wait for the response on a
reply channel:

```
connection goroutine ──request──▶ event goroutine ──▶ registry ──▶ harmony
        ▲                              │                              │
        └──────────response────────────┘                        renderer
```

Nothing locks the musical state, because nothing else touches it. This is the
concurrency contract `docs/renderer.md` documents from the other side: `Update`
arrives from one goroutine only, and a renderer that hands state to an audio
callback thread does its own locking.

Once the event goroutine stops, `handle` answers "daemon is shutting down"
rather than blocking forever on a channel nobody reads.

## Event path

`request → registry.Apply → harmony.Apply → renderer.Update → renderer.Trigger`

A renderer error is logged **and** returned as a failure response, but never
terminates the daemon and never rolls back the event. The registry and the
harmony engine have already advanced by then, so the session is tracked and the
voice released exactly as if audio had worked; the response says
`session tracked, but the renderer failed: …` so a client can tell the difference
between "your event was rejected" and "your event landed but you will not hear
it". `hum status` keeps working through a broken renderer, which is what makes
the failure diagnosable.

A rejected event never reaches the renderer, so a replayed `session.completed`
cannot sound a second completion phrase.

## Control path

| Command | Behaviour |
|---|---|
| `ping` | answers `{"ok":true}` |
| `status` | registry snapshot plus the current musical context |
| `mute` / `unmute` | proxied to the renderer |
| `volume` | proxied after the protocol's own range check |
| `theme.list` / `theme.use` | proxied to the theme loader |
| `audio.test` | triggers a two-second tone on the current root |
| `shutdown` | takes the identical path to `SIGTERM` |

`theme.use` reaches the renderer through the optional `renderer.Themeable`
interface. Keeping it off `renderer.Renderer` means a future renderer with no
notion of a theme — a Hue bridge, say — does not have to pretend to have one.
The swap retargets each sounding drone's gain, expression **and** envelope in
place, without touching envelope state or position. Rebuilding the oscillators
would fire every voice's attack at once, so a theme change would sound like every
session restarting.

`audio.test` exists for `hum doctor --audio-test`, which has to answer "is sound
physically reaching the speakers". It reports `played: false` when the renderer
is `nop` or the daemon is muted, because a test that reports success into silence
would send the user hunting for a fault in their audio hardware.

`status` reports the renderer's name, its sample rate through the optional
`renderer.Sampled` interface, and the daemon's own version, so `hum doctor` can
diagnose a device fallback and a version skew over the connection it already has.

## Musical context

The daemon holds **one** musical context at a time. `PRD.md` §7 assumes a single
key, scale and theme; §12 permits per-project configuration. With two projects
active those conflict directly, and two concurrent tonal centres would sound
like two unrelated pieces of music playing at once — the opposite of §23's
"harmonious chords".

The rule, from #49:

- A `session.started` carrying a `root` adopts that project's context **only
  when no session is sounding**.
- While anything is sounding, the established root, scale and theme persist and a
  joining session from another project inherits them.
- `status` reports `context_owner`, so which project won is never a mystery.

Roots are canonicalised with `filepath.EvalSymlinks`, so a symlinked path and its
target are one project rather than two. A `root` that is relative, missing, or
not a directory is rejected: a typo must not masquerade as "this project has no
configuration".

The daemon's own working directory never determines musical context. Under
`brew services` that directory is `$HOME`, so deriving anything from it would
silently give every Homebrew user global configuration only.

## Startup

1. Resolve global configuration, then load the theme it names.
2. Open the renderer. `--no-audio` selects `nop`.
3. Build the registry and the harmony engine from the resolved root and scale.
4. Bind the socket, then serve.

If audio initialisation reports `ErrNoDevice`, the daemon logs a warning and
continues on the `nop` renderer instead of exiting. A headless box should still
run the daemon and still answer `hum status`.

## Shutdown

`SIGINT`, `SIGTERM` and the `shutdown` command all take the same path, so
`hum stop` and a signal are indistinguishable in effect:

1. Stop accepting connections; in-flight requests drain against the still-running
   event goroutine.
2. Release every drone by pushing an empty harmony state to the renderer.
3. Wait out the theme's release envelope plus 500 ms.
4. Close the renderer, which closes the audio device.
5. Unlink the socket and the pidfile.

Killing the device mid-drone clicks or leaves a stuck buffer, which is exactly
the jarring behaviour Hum exists to avoid, so the wait is not optional. It is
bounded, and a **second** signal abandons it and exits non-zero: an operator must
always be able to force the issue.

Voices are released strictly before the renderer closes. The reverse order would
close the device while a fade was still owed, which is the click the envelope
exists to prevent.

## Supervision

`humd` is meant to run under a supervisor, and the only supervisor policy that
works is "restart on abnormal exit". A clean `hum stop` must stay stopped, or the
stop command is a no-op from the user's point of view.

Homebrew's `keep_alive` expresses this differently on each platform, so
`Formula/hum.rb` branches on `OS.mac?`:

| Platform | `keep_alive` | Generated |
|---|---|---|
| macOS | `successful_exit: false` | `KeepAlive = { SuccessfulExit = false }` |
| Linux | `crashed: true` | `Restart=on-failure` |

Neither value works on both. `crashed: true` maps to launchd's
`KeepAlive { Crashed = true }`, which restarts only a process killed **by a
signal** — and a Go program is never killed by one. The runtime installs handlers
for the fatal signals, prints a traceback, and exits **2**. Measured against
v0.1.6 under `brew services`: `kill -9` left `runs = 1`, `last exit code = 2` and
no restart, and so did `SIGABRT` and `SIGSEGV`, each after a full runtime crash
dump. A panic takes the same exit. So on launchd, `Crashed` never fires for
`humd` and the daemon simply stays dead.

`successful_exit: false` is right there — launchd restarts on any non-zero exit,
which is what a Go crash is, and `hum stop` exits 0 and is left alone. But
Homebrew's systemd translation tests `@keep_alive[:successful_exit].present?`, and
`false.present?` is false in ActiveSupport, so that branch emits **no** `Restart=`
line at all. On Linux `crashed: true` is the value that produces
`Restart=on-failure`, which is the intended semantics there.

`contrib/systemd/humd.service` covers source installs and says
`Restart=on-failure` directly. `RestartSec=5` bounds a restart loop: a daemon
respawning ten times a second on a bad config would out-log anything the daemon
itself writes.

## Reaping

A terminal session is dropped once it is older than the reap window, on a ticker
inside the event goroutine. An always-on daemon would otherwise accumulate every
session the machine ever ran. Active sessions are never reaped, however old.

## Logging

Structured `log/slog` to stderr, with no third-party logger (`PRD.md` §22).
Under a supervisor stderr is the log file, so everything the daemon has to say
goes there and nothing goes to stdout except `--version`.
