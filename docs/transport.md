# transport

`internal/transport` is the wire layer between `humd` and any number of `hum`
clients. It defines two abstractions — `Handler` and `Listener` — and provides
one concrete `Listener` over a Unix domain socket.

## One response per request

The server spawns one goroutine per accepted connection. Each goroutine reads
newline-delimited JSON objects from that connection and writes exactly one
`protocol.Response` for each object it reads. A client may hold the connection
open and batch requests: the second request is read only after the first response
is written. There is no multiplexing; request and response always alternate on a
single connection.

## Deadline strategy

A fixed deadline on the whole connection would kill a client that holds it open
between requests. Instead the deadline is reset before each read and again before
each write: `conn.SetDeadline(now + opts.Deadline)` brackets every request-side
half and every response-side half separately. A wedged client (one that stops
writing mid-request, or one that stops reading mid-response) times out within
one deadline window without affecting any other connection.

`Options.Deadline` defaults to 5 s. Callers that need longer round trips — e.g.
`shutdown`, which must drain audio — should configure a longer deadline on the
`Listener` rather than adding special-case logic inside the handler.

## Cancellation and grace period

`Serve` watches the caller's context in a dedicated goroutine. When the context
is cancelled, that goroutine closes the underlying `net.Listener`, which causes
`Accept` to return an error. `Serve` distinguishes a cancellation-driven error
(context already done) from a genuine accept failure; only the latter is returned
as a non-nil error.

After closing the listener, `Serve` waits up to `opts.Grace` for in-flight
connections to finish. A `sync.WaitGroup` tracks active connection goroutines.
The grace wait uses a `select` between the WaitGroup draining and a timer: tests
can observe completion without sleeping on a fixed timer.

The context-watcher goroutine selects on both `ctx.Done()` and a `stop` channel
that `Serve` closes on return. Without the stop channel, the goroutine would
block forever on a long-lived context that is never cancelled.

## Stale vs. live socket probe

Before binding, `NewUnixListener` checks whether the socket path already exists.
If it does it probes the path:

1. Dial with a short timeout.
2. If the dial fails with `ECONNREFUSED` or `ENOENT` — nobody is listening, or
   the file disappeared in the race between `Stat` and `Dial` — the socket is
   stale; unlink it and proceed.
3. If the dial fails for any other reason — permissions, bad fd state — something
   is holding the path but we cannot tell what. Return an error and refuse to
   start rather than blindly unlinking.
4. If the dial succeeds, send a `ping` request and read the response. Any
   complete response (including `{"ok":false}`) proves a live daemon is
   answering; return `ErrAlreadyRunning` and leave the file untouched. A
   successful dial that does not produce a response within the probe deadline
   also triggers an error and refuses to start: something is accepting on that
   path and it is not ours to remove.

`resp.OK == false` is treated as a live daemon. A daemon that answers at all —
even with an error — is still running.

## Why a relative HUM_SOCKET is rejected

`paths.SocketPath` returns the `HUM_SOCKET` override verbatim. If that value is
a relative path, `humd` and `hum` will each resolve it against their own working
directories. `humd` is typically started by `brew services` from `$HOME`; `hum`
is invoked from wherever the user's shell is. They resolve the same relative
string to two different absolute paths and silently talk to different (or
nonexistent) sockets. Absolutising at startup does not help because each side
would absolutise against its own cwd and arrive at a different absolute path.

`NewUnixListener` therefore rejects any non-absolute path with `ErrRelativeSocket`.
The rule is: configure an absolute path or use the default.

## Why the parent directory is not chmod'd unconditionally

`paths.EnsureRuntimeDir` (and our own `os.MkdirAll` call) create new directories
with mode `0700`. They do not tighten an existing directory.

With `HUM_SOCKET=/tmp/humd.sock` the parent is `/tmp`. Calling
`os.Chmod("/tmp", 0700)` would either silently break a world-accessible system
directory or fail `EPERM` and block startup. The socket itself is locked down to
`0600` (via `os.Chmod` after bind, because `net.Listen` honours the process
umask). The parent's mode is the OS's concern; we own only the files we create.

## Pidfile contract

`NewUnixListener` writes `os.Getpid()\n` to `humd.pid` beside the socket. The
pidfile is created after the socket is bound and chmoded; it is removed by
`Close` only when its contents still equal the current PID.

`Close` also identity-checks the socket before removing it: it captures
`os.FileInfo` of the socket at bind time and only calls `os.Remove` when
`os.SameFile` confirms the path still refers to the same inode. Combined with
`net.UnixListener.SetUnlinkOnClose(false)` (which disables Go's own
`os.Remove` on close), this ensures that a second daemon which rebound the same
path after the first was `SIGKILL`'d will not have its socket removed when the
first daemon's deferred cleanup runs.

A stale pidfile (left by a `SIGKILL`) does not prevent the next start because
`NewUnixListener` removes both the stale socket and any pidfile in the same
directory before binding.

`Close` is safe to call any number of times; `sync.Once` ensures a single
cleanup pass.
