# Hum

Auditory display daemon. `humd` owns audio and session state; `hum` is a thin
client over a Unix socket. `go.mod` requires exactly two things: `gopkg.in/yaml.v3`
for configuration and themes, and `github.com/ebitengine/oto/v3` for audio
output. A third needs a strong argument.

## Commands

| Task | Command |
|---|---|
| Format, vet, test | `mise run check` |
| Coverage gate | `mise run coverage` |
| Build both binaries | `mise run build` |

`mise` pins the Go toolchain, so CI and local builds cannot drift. Run tasks
through it rather than calling `go` directly.

## Comments

**Write none.** No doc comments, no package docs, no "why" comments, no TODOs.
Compiler directives (`//go:build`) are not comments and may stay.

No exceptions, no justifications. If code needs explaining, rename something or
extract a function whose name is the sentence you were about to write.

Functionality goes in `docs/`. Reasons go in the commit message. Rejected
alternatives go in the issue or PR. Traps go in the list below.

## Tests

Every test defends an observable contract and fails on a plausible bug. Test
behaviour, boundaries and error paths — not plumbing.

- Coverage floor is 98%, enforced by `mise run coverage` and required on
  `master`. The number lives once, in `mise.toml`; CI publishes it as the
  `coverage/total` status.
- `internal/infra` asserts the build tooling itself: mise tasks, CI workflow,
  `.gitignore`. Anything duplicated across `mise.toml`, `.goreleaser.yaml` and
  the Homebrew formula gets an assertion there, because those three cannot share
  a definition and will otherwise drift apart silently.
- Exit codes and `main` wiring are asserted against the built binary. `go run`
  is useless for this: it reports 1 for any non-zero exit.
- Package-global seams (`exit`, `absolute`) exist so unreachable failures can be
  staged. Register restoration with `t.Cleanup` *before* installing a stub. No
  test in those packages calls `t.Parallel`, and adding one would race the stub.

## Traps

Things the code cannot say, that will be "fixed" back if forgotten.

- `volume` bounds read `!(v >= 0 && v <= 1)`. Every comparison against `NaN` is
  false, so `v < 0 || v > 1` accepts `"NaN"`.
- `cmd/hum` parses flags in a loop. Go's `flag` stops at the first positional,
  so a single pass reads `theme use --json minimal` as a theme named `--json`.
- `filepath.Abs` fails only when the working directory is removed, and macOS
  keeps resolving a removed one. That is why `absolute` is a variable.
- Socket paths must fit `sun_path`: 104 bytes on macOS, 108 on Linux.
  `NewUnixListener` rejects longer ones, because the kernel only says "invalid
  argument". A test socket therefore cannot live in `t.TempDir()` — the test name
  is part of that path and overflows it. Use a short `os.MkdirTemp("", "hd")`.
- oto v3.4.0 is CGO-free on macOS but declares `#cgo pkg-config: alsa` on Linux,
  so the Linux legs install `libasound2-dev` and `pkg-config`. #39 removes this.
- A fork's `GITHUB_TOKEN` is read-only. The `coverage/total` status is display
  only; requiring it would block every external contribution.
- A decoder returning `ErrMessageTooLarge` cannot resynchronise. Close the
  connection.
- Voices are released before `renderer.Close`, never after. Closing first cuts
  the fade the release envelope exists to produce.
- The event goroutine must outlive `transport.Serve`. Requests still in flight
  during shutdown are answered by it, so stopping it first deadlocks the drain.
- `renderer.Options.Volume` is never defaulted. `internal/config` decodes into
  pointers so a configured `volume: 0` survives; substituting a theme's drone
  gain, or 0.6, throws that away and unmutes a user who asked for silence.
- `Request.MarshalJSON` refuses to encode an invalid request, so the daemon's own
  rejection paths cannot be tested through the typed client. Write raw JSON lines
  (`sendRaw` in `cmd/humd`).
- The runtime coalesces rapid duplicate signals. A test that sends two `SIGTERM`s
  back to back sees one; wait for the daemon's "waiting for voices to fade" line
  in between.
- `humd` installs its signal handler before it opens the renderer or the socket.
  Notifying after the listener leaves a window where `SIGTERM` kills the process
  with the default disposition — no drain, no fade, socket left behind — and a
  test that stops a daemon the moment it accepts a connection lands in it.
- `hum` commands that persist a setting ask the daemon first and write the config
  second. The reverse order leaves a file claiming `muted: true` while sound
  keeps playing.
- `config.Patch` decodes into `yaml.Node`, not `Config`. Decoding into the struct
  and re-encoding deletes comments and every key the struct does not model, so
  `hum volume` would strip the scale and theme lists `hum init` wrote.

## Protocol

`internal/protocol` is a published contract. Nothing in it may name an AI tool,
agent framework or client. The wire shape does not follow Go's structs — an
event request stays flat. The event-type set is closed and `Validate` rejects
anything outside it, so growing it is a protocol change; unknown JSON *fields*
are ignored, which is what makes adding a field additive.
