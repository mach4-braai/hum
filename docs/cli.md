# CLI

`hum` is a thin client. It parses arguments, resolves what only the client can
know, sends one request over the Unix socket and renders the answer. Every
decision about sound belongs to `humd`.

## Commands

| Command | Purpose |
|---|---|
| `hum init` | write a project configuration |
| `hum start` | announce a new work session |
| `hum stop` | stop the daemon |
| `hum complete` / `hum fail` / `hum cancel` | end a session |
| `hum update` | report progress without ending a session |
| `hum status` | report daemon and session state |
| `hum mute` / `hum unmute` | silence and restore output |
| `hum volume [N]` | report or set the output level |
| `hum theme list` / `hum theme use NAME` | inspect and switch themes |
| `hum doctor` | diagnose the installation |
| `hum ping` | check that the daemon is reachable |

## Ending a session

`complete`, `fail` and `cancel` are one code path, `sendTerminal`, differing only
in the event they emit. They resolve `--id` then `$HUM_SESSION_ID` and, unlike
`start`, refuse to invent one: a generated id would name a session the daemon has
never heard of. Without an id they exit 2.

A session that is unknown or already terminal exits **1** with the daemon's own
message, so a CI script wrapping real work cannot mistake a lost session for
success. `fail` means the work failed; Hum failing to reach the daemon is exit 3.
`cancel` means the work was abandoned, which is not an error condition, and emits
no cadence.

`update` shares that id resolution but is not terminal: it never ends a session,
repeated calls are expected, and `--meta agents=N` is the key the expression
engine reads for stereo width. Other keys merge into the session's metadata and
the engine ignores them.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | the daemon returned an error, or a local write failed |
| 2 | usage error |
| 3 | the daemon is unreachable |

The split matters in scripts: 2 means the command never reached the socket, so
nothing happened, while 1 means the daemon was asked and refused. Usage errors
are detected before dialling for exactly that reason — `hum volume 1.5` must not
leave a daemon half-configured.

`hum stop` deliberately exits **0** when no daemon is running, matching
`systemctl` and `brew services`, so it is safe in unconditional teardown.

## Dispatch

Commands register themselves:

```go
func init() {
	register("status", runStatus)
}
```

The alternative — a table in `main.go` listing every handler — makes each new
command a two-file change and gives merge conflicts to anything adding a command.
`register` panics on a duplicate name, because a silently shadowed command would
misroute work with no error. `hum help` text stays hand-written: it documents the
CLI a user should expect, including commands still being built, which a table
generated from the registry could not.

## Flags may appear anywhere

Go's `flag` stops at the first positional argument, so a single pass reads
`hum theme use --json minimal` as a theme named `--json`. `operandsOf` therefore
loops: parse, take the first positional, parse the remainder, repeat. Commands
bind their own flags through a callback that runs on every iteration, so a
default must be seeded from the current value to survive the loop, and a
repeatable flag such as `--meta` accumulates into one map.

## `--json`

`--json` prints the command's payload verbatim when the command has one
(`status`, `theme list`, `volume` with no operand, `doctor`), and the raw
response envelope when it does not (`ping`, `mute`, `volume N`, `stop`,
`theme use`). Payload-first output is what a script wants: `hum status --json |
jq '.sessions[].pitch'` rather than reaching through `.data` every time. The
schemas are in `docs/protocol.md`; `doctor` is the exception, being a client-side
report rather than a daemon payload.

## Session identity

`hum start` resolves the session id from `--id`, then `$HUM_SESSION_ID`, then a
generated short random id, and prints the result as its only stdout line, so
`id=$(hum start --title build)` works. Printing happens only after the daemon
accepts the event: an id for a session that does not exist would send later
`hum complete` calls at nothing.

## The project root travels over the wire

`session.started` carries the canonical absolute project root, because the client
is the only process that knows it. Under `brew services` the daemon's working
directory is `$HOME`, so a daemon that resolved config from its own working
directory would silently ignore every project's `.hum/config.yaml` — the default
Homebrew install. `paths.ProjectRoot` walks up for `.hum/config.yaml`, falls back
to the git root, then the working directory, and resolves symlinks so one project
reached by two paths stays one project.

`--workspace` is a free-text label, not a path, and is never used to locate
configuration.

## Persisted settings

`hum mute`, `hum unmute`, `hum volume N` and `hum theme use` write the setting to
the **global** config after the daemon accepts the change, so the state survives
a restart. Writes go through `config.Patch`, which preserves unrelated keys and
comments and replaces the file atomically; see `docs/configuration.md`. A failed write
is exit 1 even though the daemon already applied the change, because the user's
intent — a durable setting — was not achieved.

The daemon is asked first and the file second. The reverse order would leave a
config claiming `muted: true` while sound kept playing.

## `hum status`

Columns are `ID WORKSPACE TITLE STATE NOTE AGE`. NOTE is the pitch the session's
drone is sounding, which is what lets an operator match a note they can hear to
the work causing it; a session with no voice shows `-`.

Titles are truncated only when stdout is a terminal. The width comes from a
`TIOCGWINSZ` ioctl, which fails with `ENOTTY` on a pipe, so one call answers both
"how wide" and "is this a terminal" — and piped output stays lossless, because a
log or a `jq` pipeline must not silently lose characters.

An empty registry prints `no active sessions` and exits 0. Absence of work is not
an error.

## `hum stop`

`stop` sends `shutdown` and then polls until the socket file disappears, within
`--timeout` (10s by default, rather than the 2s used for a request). Returning
before the daemon has gone would make `hum stop && humd` race a still-bound
socket. Timeout expiry is exit 1 with the socket path in the message.

## `hum doctor`

`doctor` prints a `pass`/`warn`/`fail` row per check and exits non-zero if any
check fails. It runs every check it can **without** a daemon, because a user
whose daemon will not start is exactly who needs it, and Homebrew users have no
other diagnostic.

Warnings do not fail the run. A version skew between `hum` and `humd`, or a
silent renderer, are conditions worth reporting but not reasons to exit non-zero.
The audio row separates a deliberate `humd --no-audio` from a fallback to the
silent renderer on a machine with no device: both run `nop`, but one is what the
operator asked for and the other is a fault, and telling a headless daemon it has
no audio device sends the user hunting for a hardware problem that does not exist.

The config rows carry provenance, so a user can see which layer set `music.root`
rather than guessing between four of them. The `music` row renders the resolved
pitch — `root C4, scale major` — not the bare note class, because the
class alone cannot say which register the drone will sound in, and the register is
the setting a user most often wants to check. `--audio-test` plays a two-second
tone through the daemon and says plainly when nothing could be heard, since a
diagnostic that reports success into silence is worse than no diagnostic at all.

The supervisor row answers the first question in every bug report: whether `humd`
is running, who started it, and where its log is. It probes `launchctl print` for
the Homebrew label and `systemctl --user is-active humd`, and reports "started
manually" when neither answers — a foreground `humd` is a legitimate way to run it,
so that is not a failure. The log path is derived from the executable's own
location: a `hum` at `<prefix>/bin/hum` implies
`<prefix>/var/log/hum/humd.error.log`. That is the **error** log deliberately —
`humd` writes its structured log to stderr, which the formula's `error_log_path`
routes there, so `humd.log` beside it is the stdout file and normally empty.
`internal/infra` asserts the formula and the path agree. It is printed even with no
daemon, because a crash log is exactly what a user needs then, and a binary outside
a `bin/` directory has no prefix to derive, so the row says so rather than inventing
a path.

## `hum init`

`init` writes `.hum/config.yaml` (or the global file with `--global`), defaulting
`project.name` to the project directory's base name. The generated file lists the
valid scales and themes as comments, taken from `harmony.ScaleNames()` and
`theme.List()`, so the file cannot document names the build does not support.

It refuses to overwrite without `--force` and prints the path either way.
`--print` writes the document to stdout and touches nothing.
