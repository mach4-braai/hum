# Integrations

Hum knows nothing about any specific tool. That is deliberate: the protocol
is line-oriented JSON over a Unix socket, and every integration is the same
three moves — emit `session.started`, run the work, emit a terminal event.
This guide shows how to wire those moves into shell wrappers, git hooks and
CI steps, and how to ensure that an absent daemon never breaks the thing being
wrapped.

## The shape of an integration

One work session maps to one sustained drone. An integration's entire job is:

1. Emit `session.started` with a stable `id`.
2. Run the work.
3. Emit exactly one terminal event — `session.completed`, `session.failed` or
   `session.cancelled` — carrying the same `id`.

The terminal event is what releases the drone. The daemon tracks sessions
by id; it has no visibility into the process on the other side of the socket,
so if that process exits without sending a terminal event, the drone continues
sounding indefinitely. There is no timeout and no heartbeat mechanism.

That makes the terminal event the single most important correctness property of
any integration. A wrapper that can exit without sending one — because a
command failed, because the user hit Ctrl-C, because the machine received
SIGTERM — leaks a voice. Every recipe below uses a `trap` to close this gap.

### Event fields

| Field | Type | Notes |
|---|---|---|
| `event` | string | one of the five event types below |
| `id` | string | required; bounded at 128 bytes |
| `title` | string | human-readable label for this session |
| `workspace` | string | free-text grouping label |
| `root` | string | absolute path to the project root |
| `priority` | number | relative scheduling hint |
| `metadata` | object | arbitrary string key/value pairs |

The five event types are a closed set: `session.started`, `session.updated`,
`session.completed`, `session.failed`, `session.cancelled`. The daemon rejects
anything outside that set with `ErrUnknownEvent` rather than silently ignoring
it, so a typo surfaces immediately.

Every optional field is omitted when empty. The protocol ignores unknown JSON
fields on receipt, so adding a `metadata` key is backwards-compatible.

## Sending events from the shell

No Go client is needed. `nc -U` ships with macOS and is sufficient:

```sh
printf '{"event":"session.started","id":"a1","title":"build"}\n' \
    | nc -U "${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}"
# {"ok":true}
```

Batching works down one connection; each request receives its own response
line in order:

```sh
printf '%s\n%s\n' \
    '{"event":"session.started","id":"a1","title":"build"}' \
    '{"event":"session.completed","id":"a1"}' \
    | nc -U "${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}"
# {"ok":true}
# {"ok":true}
```

`socat` is an equivalent where it is installed:

```sh
printf '...\n' \
    | socat - "UNIX-CONNECT:${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}"
```

Subsequent examples use `${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}` as
the socket path. Setting `HUM_SOCKET` in the environment overrides it for the
whole process tree without modifying any recipe.

## Shell wrapper

The wrapper below emits `session.started`, runs an arbitrary command, then
sends `session.completed` or `session.failed` based on the exit status.
Critically, it also traps `INT` and `TERM` to send `session.cancelled` when
the work is interrupted before it finishes.

```sh
#!/bin/sh
# hum-wrap — wrap a command in a hum work session.
# Usage: hum-wrap TITLE COMMAND [ARG...]
#
# Requires the hum binary.  Degrades silently when the daemon is not running:
# the wrapped command still executes, without audio feedback.

_hum_title="$1"; shift
_hum_sock="${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}"
_hum_id=""

# Send a terminal event by id, then clear the id to prevent a second send
# from a concurrent trap.  No-ops if the socket is gone or nc fails.
_hum_terminal() {
    [ -n "$_hum_id" ] || return 0
    [ -S "$_hum_sock" ]  || return 0
    _msg_id="$_hum_id"
    _hum_id=""
    printf '{"event":"%s","id":"%s"}\n' "$1" "$_msg_id" \
        | nc -U "$_hum_sock" >/dev/null 2>&1 || true
}

# hum start exits 3 when the daemon is unreachable.  In that case _hum_id
# stays empty and every subsequent send is a no-op, so the wrapped command
# still runs without interference.
_hum_id=$(hum start --title "$_hum_title" 2>/dev/null) || _hum_id=""

# Establish traps immediately after start so no interrupt goes unguarded.
#
# INT and TERM mark the session cancelled and exit; the exit triggers the EXIT
# trap, which sees an empty _hum_id (cleared by _hum_terminal above) and
# sends nothing — preventing a double send.
#
# EXIT fires on every remaining exit path and picks completed or failed based
# on the child's exit code.
trap '_hum_terminal session.cancelled; exit 130' INT TERM
trap '_ec=$?
    if [ "$_ec" -eq 0 ]; then
        _hum_terminal session.completed
    else
        _hum_terminal session.failed
    fi' EXIT

"$@"
```

### Why the trap must cover both INT/TERM and EXIT

The `EXIT` trap alone is not enough. When the shell receives `SIGINT`, it runs
the `INT` trap (if any), then the `EXIT` trap. Without a dedicated `INT` trap,
the `EXIT` trap fires with the child's partial exit code and sends
`session.failed` when the operator cancelled the work deliberately —
`session.cancelled` is the semantically correct event. More importantly, on
some shells a bare Ctrl-C skips the `EXIT` trap entirely if the child caught
and re-raised it; covering both signals closes that gap.

### Session id generation

`hum start` generates a random id internally and prints it to stdout — that
is why the assignment above uses command substitution. If you need to generate
an id before calling `hum start` (for example to embed it in log output), a
PID combined with the epoch in seconds is unique per invocation and portable:

```sh
_id="${$}$(date +%s)"
```

The protocol bounds `id` at **128 bytes**. This value is well under that
limit: a PID is at most seven digits on Linux, and a Unix epoch timestamp is
ten, giving at most seventeen bytes total. An id that exceeds 128 bytes is
rejected with a validation error, not truncated — the daemon prefers a clean
failure to silently mangling a session identifier.

## Git hook

A `pre-push` hook is the right place to wrap validation work. It fires after
the local commits are ready but before the push is sent to the remote, so it
can abort the push on failure while still representing the work as a hum
session. A `post-commit` hook fires after the commit has already landed and
cannot abort anything, which makes it unsuitable for work that needs to gate
the git operation.

Save the following as `.git/hooks/pre-push` and make it executable
(`chmod +x .git/hooks/pre-push`):

```sh
#!/bin/sh
# pre-push hook — wrap the test suite in a hum session.
# The hook runs the project's test command; adjust the last line as needed.

_hum_sock="${HUM_SOCKET:-${HUM_HOME:-$HOME/.hum}/humd.sock}"
_hum_id=""

_hum_terminal() {
    [ -n "$_hum_id" ] || return 0
    [ -S "$_hum_sock" ]  || return 0
    _msg_id="$_hum_id"
    _hum_id=""
    printf '{"event":"%s","id":"%s"}\n' "$1" "$_msg_id" \
        | nc -U "$_hum_sock" >/dev/null 2>&1 || true
}

_hum_id=$(hum start --title "pre-push" 2>/dev/null) || _hum_id=""

trap '_hum_terminal session.cancelled; exit 130' INT TERM
trap '_ec=$?
    if [ "$_ec" -eq 0 ]; then
        _hum_terminal session.completed
    else
        _hum_terminal session.failed
    fi' EXIT

# Replace with the project's own test or validation command.
make test
```

If the test command exits non-zero, the hook exits non-zero, aborting the
push and sounding the failure phrase. The daemon does not know or care that
git is involved; it sees only a session that started and failed.

To share the hook across contributors without committing it, put it at a
project path and point git at the directory:

```sh
git config core.hooksPath .githooks
```

## CI integration

A cloud CI runner has no audio device and typically no running `humd`. Sending
events to such a runner produces no sound. The pattern is therefore useful
only for **self-hosted runners** that run on a developer's own machine or a
local server where `humd` is already running.

For a self-hosted runner, no special configuration is needed: the socket path
resolves from the runner's environment variables exactly as it does locally.

```yaml
# .github/workflows/ci.yml  (self-hosted runner only)
jobs:
  build:
    runs-on: self-hosted
    steps:
      - uses: actions/checkout@v4
      - name: Build and test
        run: ./hum-wrap "CI ${{ github.run_id }}" make test
```

On a cloud runner the `hum start` call inside `hum-wrap` exits 3 (daemon
unreachable), `_hum_id` stays empty, and the rest of the script runs
normally — `make test` still executes and the workflow result is unaffected.
The recipe is safe to leave in place for mixed environments: it sounds on
local runners and is silent elsewhere.

If you want to forward events from a cloud runner to a local daemon, mount the
socket over an SSH tunnel before the job runs:

```sh
ssh -fNL /tmp/humd-ci.sock:~/.hum/humd.sock dev-machine
export HUM_SOCKET=/tmp/humd-ci.sock
```

This is an advanced pattern; the socket must be accessible to the runner
process and cleaned up after the job. Absent that plumbing, the degradation
described above is the right answer.

## Degrading safely

Every recipe above guards against a missing daemon with `[ -S "$_hum_sock" ]`
— a POSIX test that succeeds only when the path exists and is a Unix socket.
Sends are also wrapped in `>/dev/null 2>&1 || true`, so even if the socket
disappears between the guard and the write, the script continues.

`hum start` is still preferable to a raw `nc` command, because its exit code
separates "the daemon refused the event" (exit 1) from "the daemon is not
running" (exit 3). Note that `_hum_id=$(hum start …) || _hum_id=""` does **not**
preserve that distinction: `||` fires on every nonzero status, so it absorbs
exit 1 and exit 3 alike and leaves `$?` at 0. That is the right trade-off for a
wrapper whose only job is to stay out of the way. A wrapper that needs to tell
the two apart must capture the status explicitly:

```sh
_hum_id=$(hum start --title "$1" 2>/dev/null)
_hum_status=$?
[ "$_hum_status" -eq 0 ] || _hum_id=""
```

An integration that breaks a developer's build when `humd` is not running is
worse than no integration — it teaches developers to remove the hook rather
than tolerate the noise. Degradation to a no-op is not a fallback; it is the
correct default behaviour.

## Choosing workspace, title and root

Three fields label a session; they serve different purposes:

| Field | Semantics | Config resolution |
|---|---|---|
| `title` | Human-readable description of the specific task | No effect |
| `workspace` | Free-text grouping label, shown in `hum status` | No effect |
| `root` | Absolute path to the project root | Yes — locates `.hum/config.yaml` |

`root` is the field that matters for musical context. When a session carries
a `root` and no other session is currently sounding, the daemon walks up from
that path looking for `.hum/config.yaml`, adopts its `music.root`, `scale` and
`theme`, and sets `context_owner` in the status output. A joining session from
a different project inherits the established context rather than overriding it.

`hum start` resolves `root` automatically from the working directory (via
`paths.ProjectRoot`), so an explicit `--root` is rarely needed.

`workspace` is useful in a multi-repo setup where several projects are active
at once and you want `hum status` to show which repository each session belongs
to. Set it to the repository name:

```sh
hum start --workspace myrepo --title "build"
```

This has no effect on which musical configuration is used — only `root`
governs that. The distinction is worth stating explicitly: two sessions from
different workspaces but the same `root` share a musical context; two sessions
from the same workspace but different `root` paths do not.
