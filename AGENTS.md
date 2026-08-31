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

- Every statement in the module runs under test, and `mise run coverage` enforces
  that twice. The percentage is the weaker half: `go tool cover -func` prints one
  decimal, so with 2,233 statements a single uncovered one is 99.955% and still
  prints `100.0`. The real gate is the block scan over `coverage.out`, which fails
  on any block whose aggregated hit count is zero. Aggregation matters — a block
  appears once per test binary, covered in one section and zero in another, so the
  counts must be summed per block key before judging.
- `UNEXERCISABLE` in that task is the single exemption: the `newOtoPlayer` block in
  `internal/audio/format.go`, which opens a real audio device and cannot run in CI.
  It is keyed on the start line, so moving that function fails the gate until the
  pattern is updated. That is deliberate — an exemption nobody notices is a hole.
- Coverage is visible from the README: a live CI badge beside a Codecov badge for
  `master`. The `coverage` job uploads `coverage.out` with `codecov/codecov-action`.
- Codecov reads lower than the gate and that is expected, but not for the reason a
  quick guess gives. Codecov counts **lines**, where `go tool cover` counts
  statements, and it counts every line in the profile — including the
  `newOtoPlayer` body that `-func` cannot see and `UNEXERCISABLE` excuses, and the
  `case <-d.shutdown:` arm in `cmd/humd/daemon.go`, which carries zero statements
  and so cannot move the local figure at all. Measured: 2,854 of 2,859 lines,
  99.82%, five misses across two files, against the gate's 100.0% of 2,233
  statements. Do not "fix" that gap by targeting 100% in `.github/codecov.yml` —
  the project status would be red on every commit. It targets `auto` with a zero
  threshold, which forbids a regression.
- The patch target is 100%, and that is a **second, stricter policy rather than a
  restatement of the gate**. It is line coverage, so it can reject a change the
  statement gate accepts: `case <-d.shutdown:` is the standing example, a line
  carrying zero statements that `mise run coverage` is blind to and Codecov counts
  as a miss. A change touching lines of that shape turns `codecov/patch` red while
  `check` and `coverage` stay green. That is the intended trade — decide on the
  evidence in the Codecov comment, and lower the patch target rather than pretend
  the two measures agree.
- `use_oidc` is switched off for a pull request from a fork, matching Codecov's own
  example. A fork build is not granted `id-token: write`, so asking for OIDC there
  would yield an empty token and fall back anyway; the documented fork path is the
  tokenless one below.
- A pull request from a fork uploads without a token: `codecov-action` v4 and later
  rewrites the branch to an unprotected `forkname:branch` ref, which Codecov accepts
  tokenlessly for a public repository. This has nothing to do with `GITHUB_TOKEN`
  being read-only on a fork — that restriction governs the `coverage/total` status,
  which is why the step publishing it is skipped for fork pull requests. Codecov
  uploads never use `GITHUB_TOKEN`.
- A push to `master` is different. Codecov treats a real branch as protected and
  refuses an unauthenticated upload, so the step authenticates with OIDC —
  `use_oidc: true` plus `id-token: write` on the job — rather than a stored secret.
  Any token supplied alongside it would be ignored. codecov-action#1817 is the
  cautionary tale and not a reason to avoid this: that report ran v5.4.0 without
  `id-token: write`, logged `Token of length 0 detected`, fell back to a tokenless
  upload and was rejected. Codecov closed it as fixed in 5.4.3, and the pin here is
  v7.0.0. The permission is the load-bearing half; dropping it silently returns to
  that failure.
- `fail_ci_if_error` is false, because the block scan has already enforced coverage
  before anything is uploaded and a third party being unreachable must not turn a
  correct build red. The consequence is that a rejected upload is quiet, so the
  README badge reading `unknown` is the signal that `master` uploads are not
  authenticated — check the Codecov step in the run before assuming the badge is
  merely stale. `coverage/total` remains the per-commit figure that does gate.
- `go tool cover -func` counts only statements inside function declarations, so the
  body of a package-level `var f = func(){}` is invisible to it. That makes a seam
  written as `var x = func(){ … }` a way to hide code from the counter rather than
  test it, and the block scan sees through it anyway. Write the real work as a
  function declaration and point a `var` at it: `terminalColumns` and
  `readTerminalColumns` in `cmd/hum/width_unix.go` are the shape to copy, and
  `setConnDeadline`, `runCommandQuietly`, `newYAMLEncoder` and `openAudioRenderer`
  all follow it.
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
- `mise run mutate` asks the second question coverage cannot: 100% of statements
  says every line ran, not that any assertion would notice a line changing meaning.
  It mutates `internal/harmony` and `internal/protocol` — the two packages that
  carry logic rather than wiring — and takes about eight minutes for 573 mutants.
  It is a local task, not a CI job: the runner cost is one `go test` per mutant, so
  the same 573 compilations that take eight minutes here would add tens of minutes
  to every pull request.
- The gate is the survivor count, never the percentage. `go test` exits 1 for a
  mutant that fails a test, for one that never compiled and for one that hung until
  its budget ran out, and go-mutesting reads exit 1 as a kill — 47 of the 573 never
  compile, and its `TimeOutCount` is never assigned, which is why every run of the
  built-in runner reports zero timeouts. `scripts/mutate-exec.sh` runs the tests
  itself and separates the three, so `EQUIVALENT` and `HUNG` in `mise.toml` are
  exact expectations: too many is a test that stopped noticing, too few is a number
  to lower. A mutant that never compiles is not a behaviour change at all, so its
  count is reported and not gated.
- The 25 survivors cannot be killed by a test, only by contorting the code they
  live in. They are: values returned beside a non-nil error that every caller
  discards; comparisons whose boundary value no input can produce (`base > 11` in
  `parseNoteClass` is reached only for two-character notes, and no letter and
  accidental combine to 11); a comparator relaxed to `<=` where every key it
  compares is distinct; an array one element longer than any index used; assigning
  a value the variable already holds; and the placeholder in `Release`, overwritten
  by the shift before it is read.
- The three hangs are one condition: relaxing `errors.Is(err, io.EOF) && len(line)+
  len(chunk) > 0` in `Decoder.Decode` turns a clean EOF into an empty line, which
  the loop discards and reads again forever. Nothing can make that fail faster than
  the timeout, because the mutant never returns.
- Four mutants looked equivalent and were not, which is the reason to read the
  survivor list rather than trust it. Dropping `isCapped` in `Acquire` leaves a
  capped session returning a degree it never took, and output is identical — but
  `free` then grows without bound, so the invariant is asserted directly.
  `sort.SliceStable` relaxed to `<=` reverses degrees of equal consonance, which a
  scale carrying an interval of 12 produces. And the `octave > 9` guard is redundant
  for accept-or-reject, so both mutants of it survive until a test reads the message
  it exists to produce.

## Traps

Things the code cannot say, that will be "fixed" back if forgotten.

- `volume` bounds read `!(v >= 0 && v <= 1)`. Every comparison against `NaN` is
  false, so `v < 0 || v > 1` accepts `"NaN"`.
- `voicing` does not lift every harmony. A third keeps its scale octave and a
  sixth drops one, so the default C4 `major` install sounds A3, C4, E4 — the
  relative minor — before it sounds anything else. Restoring the unconditional
  `.Transpose(12)` is a one-character change that silently deletes the chord and
  moves two of the first three voices an octave. `intervalRank` already ranks
  those four classes first; the directions in `voicing` are what make the ranking
  audible rather than merely ordered.
- Because a sixth sounds *below* the root, voices are no longer monotonically
  ascending in allocation order.
  `TestConcurrentVoicesAreNeverASecondApart` therefore compares absolute
  distance. Restoring the signed `sounding[j] - sounding[i]` makes every
  downward interval pass unchecked, and the test then proves nothing about the
  one voice placed below the drone.
- `cmd/hum` parses flags in a loop. Go's `flag` stops at the first positional,
  so a single pass reads `theme use --json minimal` as a theme named `--json`.
- go-mutesting writes each mutant over the source file it came from, runs the
  package's tests, and puts the file back. `mise run mutate` therefore copies what
  `git ls-files` reports into a temp directory and runs there; the binary invoked by
  hand in the checkout leaves a mutated source file behind the moment it is
  interrupted.
- The fork is not interchangeable. `gremlins v0.5.1` times out on 114 of 116 mutants
  in `internal/harmony` at ten times its budget (#90, #91); `zimmski/go-mutesting`
  panics in its 2019 `x/tools` loader before it mutates anything; `ooze` runs, but
  it is a library — five new `go.mod` entries against a module that allows two.
  `avito-tech/go-mutesting` has never tagged a release, so the pin is a commit
  pseudo-version and `go list -m -versions` offers nothing to bump.
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
- `golang.org/x/sys` is held at v0.44.0, well ahead of anything `purego` asks for,
  because every earlier version carries GO-2026-5024 in `NewNTUnicodeString`. That
  is the only reason `go.mod` says `go 1.25.0`: v0.44.0 requires it. Relaxing the
  directive means relaxing the pin, and `mise run vuln` will not object — the symbol
  is unreachable from this code, so govulncheck stays clean at either version. It
  was Scorecard's OSV scan that saw it, because OSV matches version ranges rather
  than call graphs, and Dependabot never offered it: an indirect dependency with no
  matching GitHub advisory.
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
- Any Linux job that *loads* the packages, not just builds them, needs
  `libasound2-dev` and `pkg-config` for the same oto reason. `govulncheck ./...`
  fails at package loading without them — `could not import C (no metadata for C)`
  — which reads like a scanner bug and is really the missing ALSA headers. The
  `fuzz` job does not need them because `internal/protocol` imports no audio.
- `TestUnstampedBuildReportsAModuleVersion` fails on a checkout that is a git
  submodule, and only there. Go derives the main module's version from VCS, and
  inside a submodule it resolves the superproject instead, leaving the module
  `(devel)` so the binary reports `dev`. A plain clone on the same toolchain stamps
  `v0.2.1-0.20260803062112-b46308c265fc` correctly, which is what CI does. Confirm
  with `git clone` into a temporary directory before believing the toolchain broke.
- `Decode` does not re-marshal what it decoded, and must not start. `json.Unmarshal`
  replaces each invalid UTF-8 byte with the three-byte U+FFFD, so a line at exactly
  `MaxMessageLen` can re-encode larger than the limit. `FuzzDecoder` found that and
  the tempting fix — marshal after unmarshalling and reject the overflow — is wrong
  twice over: it costs an allocation and a full serialisation on every event the
  daemon receives, and it rejects input the protocol accepts today, which closes the
  connection. Nothing re-encodes a decoded `Event`; `protocol.Encoder` has no
  production caller at all. The fuzz target therefore asserts the round trip only
  when `Encode` succeeds, and tolerates `ErrMessageTooLarge` from it.
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
- A client that dials and writes nothing is what that refusal produces, and the
  `serveResponses` stub in `cmd/hum` has already accepted the connection by then.
  Closing the listener frees a goroutine in `Accept`, never one parked in
  `Decode`, so `await` closes the accepted connections too. On POSIX the client's
  close usually lands as EOF and hides this; on Windows it did not, and the
  package sat there until the ten-minute test timeout.
- `stubDaemonGrace` and `daemonStopGrace` are 30s because every wait they bound
  returns on an event, not on the clock. The bound is only reached when the thing
  under test has already stopped, so a larger one costs a working run nothing and
  a smaller one buys a red build on a loaded runner. They are not slow tests.
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
- launchd's `KeepAlive { Crashed }` restarts `humd` for no failure at all, so
  `Formula/hum.rb` uses `keep_alive successful_exit: false` on macOS. Go handles
  the crash signals itself — traceback, then exit 2, the same as a panic — so
  launchd sees an ordinary non-zero exit rather than a crash: `SIGSEGV` on the pid
  `launchctl print` reports left `last exit code = 2` and no restart in 48s. And
  `SIGKILL` is not a signal `Crashed` covers, so `kill -9` correctly left
  `runs = 1` and no restart in 60s. #38's criterion expected a `SIGKILL` restart
  and was wrong about launchd.
- That value cannot be used on Linux. Homebrew's systemd translation tests
  `@keep_alive[:successful_exit].present?` and `false.present?` is false in
  ActiveSupport, so it emits no `Restart=` line at all; `crashed: true` is what
  becomes `Restart=on-failure`. The `service` block branches on `OS.mac?`, and
  `internal/infra` asserts both values are present.
- `cmd/hum` builds for Windows only because `statusWidth` is split across
  `width_unix.go` and `width_windows.go`; `TIOCGWINSZ` does not exist there. The
  Windows implementation returns 0, which means "unknown" and disables title
  truncation exactly as a piped stdout does.
- Go's stdlib `syscall` package defines only `WSAECONNABORTED` and
  `WSAECONNRESET` on Windows — there is no `syscall.WSAECONNREFUSED`, so
  `probe_windows.go` writes out 10061 itself. `syscall.ECONNREFUSED` exists there
  but is an `APPLICATION_ERROR`-based placeholder Winsock never returns, so
  matching it proves nothing. The liveness probe therefore goes through
  `notListening`, split across `probe_other.go` and `probe_windows.go`, rather
  than comparing errnos inline.
- The `build` and `check` tasks pin `shell = "bash -c"`. Their scripts are POSIX —
  `find`, `xargs`, `$(…)` — and mise runs task scripts under `cmd` on Windows,
  where none of that parses. Anything the `windows` job runs needs the same pin.
  The build stamps live inside `build` for the same reason: a `[tasks.build]`
  script can pin a shell, an `[env]` entry cannot, and mise evaluates `[env]`
  before every command, so `date -u +…` there breaks `mise install` itself.
- `-o bin/hum` is taken literally, so a Windows build without `$(go env GOEXE)`
  produces an extension-less file that `CreateProcess` will not run — the error
  is `executable file not found in %PATH%` for a file that plainly exists.
- `.gitattributes` pins `eol=lf`. Git for Windows checks out CRLF by default and
  `gofmt -l` then reports every file in the repository as unformatted, which is a
  symptom that names the wrong tool entirely.
- Tests that build a binary and then run it must append the executable suffix. The
  packages that do carry a local `exeSuffix`; forgetting it fails with
  `executable file not found in %PATH%` for a file that plainly exists.
- POSIX-only tests live in `*_posix_test.go` behind `//go:build !windows`, and the
  assertions Windows cannot make are abstracted rather than deleted:
  `assertFilePerm` and `assertPrivateDir` have a real body on POSIX and a
  stat-only one on Windows, so the behaviour either side of them is still checked.
  Three premises genuinely do not exist there — a directory's mode bits do not
  gate writes, an open file cannot be removed, and `FlushFileBuffers` on a pipe
  **blocks** instead of failing, which hung `internal/config` until the package
  timed out at ten minutes.
- `EnsureRuntimeDir` asks `MkdirAll` for `0700` and Windows ignores it, so the
  socket's parent is not access-restricted there. Fixing it needs ACL calls from
  `golang.org/x/sys`, which is a third dependency. Stated in `README.md` rather
  than silently tolerated.
- `os.UserHomeDir` reads `USERPROFILE` on Windows, not `HOME`. A test that clears
  only `HOME` still sees the real home; `setHome` in `internal/paths` sets both.
- `Reap(0)` cannot work on Windows: clock granularity puts a session that just
  ended at exactly `now`, and the cutoff comparison is `Before`. Tests that mean
  "everything terminal" pass a negative window.
- `protocol.Event.Validate` calls `filepath.IsAbs`, so `root` must be absolute for
  the **daemon's** platform — `/srv/x` on POSIX, `C:\srv\x` on Windows. That is
  forced: the daemon `os.Stat`s the path, so its own platform is the only notion it
  can act on. `docs/protocol.md` says so now; changing the wire format to mandate
  POSIX separators would be a contract change and needs its own issue.
- `serve` waits `<-served` **before** draining, and `transport.Options.Grace`
  defaults to 5s, so a shutdown can spend that long waiting for in-flight
  handlers before the fade even starts. A test that budgets 5s for a shutdown is
  therefore racing the shutdown path against itself; it passed only where a
  client's close was noticed instantly, and Windows was where it stopped being.
  Budget past `Grace`, and well under `releaseWait`.
- `windows/arm64` runs on `windows-11-arm`; the suite itself runs only on
  `windows-latest`, so the arm64 leg proves a real daemon starts and stops rather
  than that every test passes. `GOARCH: arm64 go vet` from the x64 runner covers
  compilation, which is not execution.
- `darwin/amd64` and `linux/arm64` are the two archives nothing executes. That is
  a wiring gap, not a platform limit: `macos-15-intel` and `ubuntu-24.04-arm` are
  available hosted runners. Say "not wired up", never "no runner exists".
- `.goreleaser.yaml`'s `release.footer` and the Platforms section of `README.md`
  say what Windows support means, and #70 requires them to agree. Neither can be
  derived from the other, so changing one means changing both.
- The Linux release builds are skipped unless `HUM_RELEASE_LINUX=1`, so
  `mise run snapshot` works on macOS where no ALSA cross-toolchain exists. The
  release workflow sets it; forgetting it ships a release with no Linux archives.
- `std_go_args` already passes `-trimpath` and adds `-s -w`. The formula spells
  out only the three `-X main.*` symbols, which `internal/infra` matches against
  `mise.toml`.
- The Go caches live at `.gocache` inside the checkout, and the cache path is
  written relative. `actions/cache` hashes the path it is given into the cache
  version, so `/home/runner/go/pkg/mod` and `/Users/runner/go/pkg/mod` are two
  caches under one key and neither restores the other; `@actions/glob` then
  refuses any pattern containing `..`, warning and archiving nothing. Relative and
  inside is the only placement left, and it costs two guards: `mise run check`
  prunes `.gocache` because gofmt walks dot-directories that `go vet ./...` skips,
  and coverage measures `$MODULE/...` because `-coverpkg` matches loaded packages
  by directory rather than by walking one — `./...` instruments every restored
  dependency and reports about 54%. Only the downloads are shared —
  `enableCrossOsArchive`, no `runner.os` in the key — since object files are built
  for one GOOS and GOARCH.
- Nothing in `release.yml` restores a Go cache. A cache restored into the job that
  publishes is the cache-poisoning path zizmor flags, which is also why every mise
  step there sets `cache: false`. The deb archive is the deliberate exception, and
  it is installed with `dpkg`, not compiled from.
- `windows/arm64` ships because it compiles CGO-free and WinMM has no architecture
  gate. Neither Windows archive has ever been executed: no runner in the matrix is
  Windows, `SIGTERM` is not deliverable there, and the liveness probe matches
  POSIX errnos Winsock does not use. #70 adds the leg that would find out.

- `mise run junit` runs the test suite independently of `mise run coverage`. Both produce test results; the coverage job runs them in sequence. `junit.xml` is written by gotestsum even when tests fail, which is why the upload step uses `if: ${{ !cancelled() }}` — the flake data is most valuable precisely when tests are red. `continue-on-error: true` on the run step keeps the job status authoritative.
- Every job in `ci.yml` must include `./.github/actions/go-cache`. `TestEveryCIJobRestoresTheGoCaches` counts `runs-on:` and go-cache references and fails if they differ. Adding a job without the composite makes the count mismatch fail that test.
- Adding an action to a workflow without recording it in `pinnedActions` (in `internal/infra/ci_test.go`) fails `TestWorkflowsPinEveryActionToAReviewedCommit`. The lead records the pin; the workflow author reports it as `owner/repo version sha`.
- `scorecard.yml` puts write permissions (`security-events: write`, `id-token: write`) at the **job** level, not the workflow level. zizmor `--pedantic` flags workflow-level write permissions as `excessive-permissions`. The workflow level carries only `contents: read`.
- `errSocketMissing` and `errSocketStale` in `cmd/hum/client.go` are declared as
  `fmt.Errorf("%w", errNoDaemon)`, not `errors.New(...)`. The wrap is load-bearing:
  `daemonStop` checks `errors.Is(err, errNoDaemon)` and exits 0 for both.
  Converting them to `errors.New` makes `hum daemon stop` exit 3 against a missing
  or stale socket, breaking the unconditional-teardown contract. `errSocketDenied`
  deliberately does not wrap `errNoDaemon` so that `hum daemon stop` exits 3 for
  `EACCES` instead.
- A session with `owner_pid > 0` but an empty `owner_host` is never probed:
  `ActiveToCancel` requires both fields and a hostname match. `hum start
  --owner-pid N` populates both automatically. Raw senders using `nc` or `socat`
  must include `"owner_host":"$(hostname)"` alongside `owner_pid`, or the pid is
  silently ignored and the session relies on traps for termination.

## Protocol

`internal/protocol` is a published contract. Nothing in it may name an AI tool,
agent framework or client. The wire shape does not follow Go's structs — an
event request stays flat. The event-type set is closed and `Validate` rejects
anything outside it, so growing it is a protocol change; unknown JSON *fields*
are ignored, which is what makes adding a field additive.
