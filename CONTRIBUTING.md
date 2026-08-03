# Contributing

Read [`AGENTS.md`](AGENTS.md) before touching build or release tooling. It
carries the comment policy, the coverage mechanics, and the list of traps the
codebase cannot express in code. The traps section is the first thing to check
when something fails in an unexpected way.

## Loop

```sh
mise run check      # gofmt, go vet, unit tests, build-tooling assertions
mise run coverage   # enforces 100% of statements by exact per-block scan
mise run e2e        # acceptance suite; macOS and Linux only
mise run vuln       # govulncheck ./...
mise run fuzz       # 30-second fuzz run against the protocol decoder
mise run mutate     # mutation testing over internal/harmony and internal/protocol
```

`mise` pins the Go toolchain, so `go` directly and `mise run` cannot drift.
Run tasks through `mise`.

`e2e` drives `SIGTERM` and does not run on Windows. `fuzz` accepts a
`FUZZTIME` environment variable if you want a longer run. `mutate` takes about
eight minutes and is not part of CI; run it when you change `internal/harmony`
or `internal/protocol`, and read `AGENTS.md` before changing the counts it
enforces. Narrowing to one file is much faster, but the binary mutates the tree
it runs in, so do it in a throwaway copy and never in your checkout:
`go-mutesting --exec "bash scripts/mutate-exec.sh" ./internal/harmony/pitch.go`.

## Rules

**No comments in Go code.** None — not doc comments, not package docs, not
inline explanations. If code needs explaining, rename something or extract a
function whose name is the sentence you were about to write. `AGENTS.md` states
the full policy and the one exemption for compiler directives.

**Coverage is gated at 100% of statements.** The gate is not a percentage
check: `mise run coverage` does an exact per-block scan over `coverage.out` and
fails on any block whose aggregated hit count is zero. Adding a code path means
testing it. The single exemption is `newOtoPlayer` in `internal/audio/format.go`,
which opens a real audio device. `AGENTS.md` explains why Codecov reads lower
than the gate and why that is expected — do not try to reconcile the two numbers.

**`go.mod` has exactly two dependencies.** `gopkg.in/yaml.v3` for configuration
and themes; `github.com/ebitengine/oto/v3` for audio. A third requires a strong
argument in the PR.

**Commit messages** use the imperative mood in the subject line, kept under
roughly 50 characters, with a blank line before the body. The body, wrapped at
72 characters, explains what changed and why — not how. Look at `git log` for
the shape in use here.

## Pull requests

Open against `master`. CI runs `check` on ubuntu, macos and windows; `coverage`;
`e2e` on posix; `vuln`; `fuzz`; and a Windows binary smoke test. A Dependabot PR
for a GitHub Actions version arrives red on purpose — it needs a SHA recorded in
`internal/infra` before it can merge; see the `pinnedActions` trap in `AGENTS.md`.

Windows support is best-effort. The suite runs there on every PR, but a failure
on a platform the change does not touch is not your bug to fix.
