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

**Comments explain why, never what.** A senior Go engineer reads the code for
behaviour. Prose that restates it goes stale, and a stale comment is worse than
none because it is believed.

Write a comment only when the code breaks a pattern a reader would otherwise
restore:

- A deviation from the obvious approach, with the reason it was rejected.
  `!(v >= 0 && v <= 1)` needs a comment; `v > max` does not.
- A constraint that lives outside the file: a platform quirk, a wire contract,
  an upstream bug, a value another file depends on.
- A correctness trap that looks like a mistake — a deliberate omission, an
  ordering requirement, an error deliberately ignored.

Delete anything else. In particular, never write a comment that:

- Restates the next line, or names what a function obviously does.
- Narrates structure (`// loop over items`, `// error handling`).
- Documents an exported symbol whose name already carries the contract. Go
  requires no doc comment, and no linter here enforces one. `NewDecoder returns
  a Decoder` earns nothing.

Reach for a better name or a smaller function before reaching for a comment.
If a workaround needs a paragraph to justify, the code is wrong — fix the code.

### Where the thinking goes instead

- **`docs/`** carries functionality: the protocol reference, wire shapes, field
  tables, user-facing behaviour. Anything a reader would call "how it works"
  belongs here, not in the source. Never document behaviour that is not yet
  implemented without labelling it as such.
- **Package doc comments** stay short — a few lines naming the package's purpose
  and the invariant a caller must not break, then a pointer to `docs/`. A
  package doc that grows section headings has become a document in the wrong
  file; move it.
- **Commit messages** carry why a change was made, and survive refactors of the
  code they describe.
- **Issues and PRs** carry the alternatives considered and rejected.

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

## Protocol

`internal/protocol` is a published contract. Nothing in it may name an AI tool,
agent framework or client. The wire shape does not follow Go's structs — an
event request stays flat. The event-type set is closed and `Validate` rejects
anything outside it, so growing it is a protocol change; unknown JSON *fields*
are ignored, which is what makes adding a field additive.
