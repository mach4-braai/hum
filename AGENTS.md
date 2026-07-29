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

Workflow YAML is the one exception, and only for what a linter reads: a
`# zizmor: ignore[...]` directive, the one-line justification beside it with a link
to the audit, and the permission comments `undocumented-permissions` requires. Those
are inputs to `zizmor`, like `//go:build` is an input to the compiler. Prose is
still prose, and still not welcome.

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
- Workflows pin actions to commits, not tags, so a moved tag cannot swap the code
  a release runs. `pinnedActions` in `internal/infra` is the one place the commit
  and its version are written down, since a bare SHA cannot say which release it
  is. Bumping one is two edits, and the assertion fails until they agree:
  `gh api repos/<action>/releases/latest --jq .tag_name` then
  `gh api repos/<action>/commits/<tag> --jq .sha`. Dependabot only makes the first
  edit, so its `github-actions` pull requests arrive red on purpose and cannot merge
  until `pinnedActions` records the commit it moved to.
- The release job installs its Linux packages from a cached deb archive, keyed on
  the runner image. `apt-get` downloads only what an image lacks, so a set
  assembled on one image can carry exact versions `dpkg -i` cannot reconcile on
  another, and it has no network to repair that. `ImageVersion` is a runner process
  variable, not something the `env` expression context is guaranteed to carry, so
  the key is built in a shell step and falls back to `$GITHUB_RUN_ID`: a missing
  image version must miss the cache, never share one.
- A cache is scoped to the ref that wrote it, and a run on one tag cannot restore a
  cache written by another tag — only the current ref and the default branch. So the
  archive is warmed by the `snapshot` job on `master` pushes and the release only
  restores it. Caching inside the release job alone writes an entry no release ever
  reads. Both call `.github/actions/linux-packages`, because a job that warms a
  different set than the release wants is a cache that never hits.
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
  so the Linux legs install `libasound2-dev` and `pkg-config`, the `on_linux`
  block in `Formula/hum.rb` depends on `alsa-lib` and `pkg-config`, and
  `.goreleaser.yaml` builds Linux only on a runner with an ALSA toolchain. #39
  deletes all four.
- A fork's `GITHUB_TOKEN` is read-only. The `coverage/total` status is display
  only; requiring it would block every external contribution.
- Every job carries `name:` spelled exactly like its id, because `zizmor --pedantic`
  wants jobs named and the `master` ruleset requires `check (ubuntu-latest)`,
  `check (macos-latest)` and `coverage` — contexts GitHub derives from the id when no
  name is given. Renaming a job to something prettier renames its status check, and
  every pull request then waits forever for one that never reports.
- A push to `master` runs `mise run snapshot` in `release.yml`, builds every target
  and publishes nothing. It exercises the build, the archives and the config; it
  cannot exercise publication, so it would not have caught either of the two burnt
  tags.
- Releases are immutable, which freezes assets and the tag the moment a release is
  published — so `.goreleaser.yaml` sets `draft: true` and uploads into a draft,
  which stays mutable. Publishing directly fails every upload with 422 and leaves a
  release with no assets, as `v0.1.2` records. A deleted immutable release frees its
  tag for deletion but the name can never be reused. `replace_existing_draft` is what
  makes a retry deterministic: GoReleaser otherwise adds a second draft for the tag
  beside the half-uploaded first.
- Publishing the draft is the promotion, and `promote.yml` listens for it. It
  subscribes to `published` alone, which is the one activity GitHub documents as
  covering publication from a draft, stable or prerelease. `prereleased` is
  documented not to fire for a prerelease published from a draft, and subscribing to
  `released` as well would risk two deliveries for one publication; the job guards on
  `!prerelease` instead. Immutability limits post-publication edits to the title and
  notes, so promoting an already published prerelease is not a path that exists.
- `promote.yml` checks out `github.event.release.tag_name`, not the default branch.
  A release published weeks after the tag would otherwise take whatever
  `Formula/hum.rb` says on `master` now.
- A workflow's `GITHUB_TOKEN` cannot write another repository, so `promote.yml`
  mints an installation token from the `homebrew-tapper` App. Its client id is
  public — `gh api /apps/homebrew-tapper --jq .client_id` needs no auth — so it is
  a variable, and `internal/infra` asserts the workflow names exactly one secret,
  the private key. The token is revoked when its job ends, so the bump has to live
  in the job that mints it. Dropping the job leaves the release green and
  `brew upgrade hum` on the old version.
- The tap's `default-branch` ruleset requires pull requests, and the bump only
  lands because `homebrew-tapper` is a bypass actor on it. That setting lives in
  the tap, so nothing here can assert it; drop it and the release fails at the
  push with "Changes must be made through a pull request".
- `GORELEASER_CURRENT_TAG` pins the release to the tag that triggered the run.
  GoReleaser otherwise takes the first of `git tag --points-at HEAD
  --sort=-version:refname`, and promoting a candidate means tagging the commit its
  candidate already tags, where that sort puts `v0.1.0-rc1` ahead of `v0.1.0`. The
  stable release then builds and publishes the candidate.
- Releases in this repository are immutable, so a publish that fails part way
  cannot be re-run over the same tag: uploading to a release that exists returns
  422. Fix the cause and cut a new tag.
- The `tap` job skips any tag containing a hyphen. `prerelease: auto` publishes
  `v0.2.0-rc1` as a prerelease, and a formula pointing at it would make a release
  candidate the default `brew install hum`.
- The bump stages the formula before comparing it. The tap has no
  `Formula/hum.rb` until the first release, and `git diff --quiet` on an
  untracked file reports no change, which would skip that first bump.
- The workflow's own concurrency group is keyed on the tag, so two releases run
  concurrently and their `tap` jobs can finish in either order. The `tap` job
  therefore takes a group of its own and refuses a version the tap already
  exceeds, compared with `sort -V` so `0.10.0` beats `0.2.0`. `git pull --rebase`
  is no defence: it replays the older bump on top of the newer one.
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
- `hum doctor` exits 1 by design with no daemon running, so the formula's `test do`
  asserts on its output with an expected status of 1. A bare
  `system bin/"hum", "doctor"` fails `brew test`.
- `cmd/hum` builds for Windows only because `statusWidth` is split across
  `width_unix.go` and `width_windows.go`; `TIOCGWINSZ` does not exist there. The
  Windows implementation returns 0, which means "unknown" and disables title
  truncation exactly as a piped stdout does.
- The Linux release builds are skipped unless `HUM_RELEASE_LINUX=1`, so
  `mise run snapshot` works on macOS where no ALSA cross-toolchain exists. The
  release workflow sets it; forgetting it ships a release with no Linux archives.
- `std_go_args` already passes `-trimpath` and adds `-s -w`. The formula spells
  out only the three `-X main.*` symbols, which `internal/infra` matches against
  `mise.toml`.

## Protocol

`internal/protocol` is a published contract. Nothing in it may name an AI tool,
agent framework or client. The wire shape does not follow Go's structs — an
event request stays flat. The event-type set is closed and `Validate` rejects
anything outside it, so growing it is a protocol change; unknown JSON *fields*
are ignored, which is what makes adding a field additive.
