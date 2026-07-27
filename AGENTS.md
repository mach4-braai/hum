# Hum

Auditory display daemon. `humd` owns audio and session state; `hum` is a thin
client over a Unix socket. `go.mod` currently has no requires; the audio backend
is the one dependency planned, so anything else needs a strong argument.

## Commands

| Task | Command |
|---|---|
| Format, vet, test | `mise run check` |
| Coverage gate | `mise run coverage` |
| Build both binaries | `mise run build` |

`mise` pins the Go toolchain, so CI and local builds cannot drift. Run tasks
through it rather than calling `go` directly.

## Comments

**Write no comments.** Not doc comments, not package docs, not "why" comments,
not `TODO`s. The only exceptions are compiler directives (`//go:build`,
`//go:generate`), which are not comments but instructions.

This is absolute. Do not reintroduce comments under any justification —
"this one is genuinely subtle", "this records a decision", "this is idiomatic
GoDoc". The answer is still no.

If code needs explaining, the code is wrong. Rename the identifier, extract a
function whose name is the sentence you were about to write, or restructure
until it reads plainly. A comment is the option you take when you have given up
on making the code say it.

Everything a comment would have said goes somewhere durable instead:

| Kind of information | Where it lives |
|---|---|
| How it works, wire shapes, field tables | `docs/` |
| Why a change was made | the commit message |
| Alternatives considered and rejected | the issue or PR |
| What a subtle line does | a named function or constant |

Comments rot silently because nothing tests them; commits, issues and `docs/`
are read in the context that keeps them honest.

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
- The default socket path must fit `sun_path`: 104 bytes on macOS, 108 on Linux.
- oto v3.4.0 is CGO-free on macOS but declares `#cgo pkg-config: alsa` on Linux,
  so the Linux legs install `libasound2-dev` and `pkg-config`. #39 removes this.
- A fork's `GITHUB_TOKEN` is read-only. The `coverage/total` status is display
  only; requiring it would block every external contribution.
- A decoder returning `ErrMessageTooLarge` cannot resynchronise. Close the
  connection.

## Protocol

`internal/protocol` is a published contract. Nothing in it may name an AI tool,
agent framework or client. The wire shape does not follow Go's structs — an
event request stays flat. The event-type set is closed and `Validate` rejects
anything outside it, so growing it is a protocol change; unknown JSON *fields*
are ignored, which is what makes adding a field additive.
