# Protocol

Hum's wire contract. It is public: third-party clients depend on it, so the
shapes here change additively or not at all. Nothing in the protocol names an AI
tool, agent framework or client.

**Implementation status.** Implemented and served. `internal/protocol` defines the
message types, framing and envelope; `hum` speaks them and `humd` answers on a
Unix socket. Per-command response payloads are specified alongside the commands
that produce them.

## Framing

Messages are single LF-terminated lines of JSON, bounded by `MaxMessageLen`
(64 KiB) excluding the newline.

A decoder that reports `ErrMessageTooLarge` has not consumed the rest of the
oversized line and cannot resynchronise, so the caller must close the connection
rather than keep reading.

Blank lines are skipped. A final line without a trailing newline is accepted, so
`printf` and a closing pipe both work.

## Transport

The socket path is `$HUM_SOCKET`, else `humd.sock` inside the config directory,
and it must fit a Unix socket address — 104 bytes on macOS, 108 on Linux. `hum`
dials it, writes one request, reads one response and closes; it never reuses a
connection, so a wedged daemon cannot leave the client holding a half-consumed
stream. The daemon serves concurrent connections, one goroutine each, and a
client may hold one connection open and batch requests down it: every request
gets exactly one response, in order.

## Events

Five session lifecycle events, a **closed set**:

| Event | Meaning |
|---|---|
| `session.started` | work began |
| `session.updated` | activity within a running session |
| `session.completed` | work finished successfully |
| `session.failed` | work finished unsuccessfully |
| `session.cancelled` | work was abandoned |

A type outside the set is rejected with `ErrUnknownEvent`, not ignored. That
error exists so a receiver can distinguish a newer sender from a malformed
message and answer each differently. Growing the set is a protocol change.

| Field | Type | Required |
|---|---|---|
| `event` | string | yes |
| `id` | string | yes |
| `workspace` | string | no |
| `title` | string | no |
| `root` | string | no |
| `priority` | number | no |
| `metadata` | object of strings | no |

Every optional field is omitted when empty, so a message carries only what the
sender actually knows.

`id` is bounded at 128 **bytes**, not runes: bytes are what cap wire size and
retained memory. An empty `id` is rejected — it would create an unaddressable
session whose drone never stops.

Unknown JSON fields are ignored. That is what makes adding a field additive.

`root` is the client's canonical absolute path to the project root, sent on
`session.started` so the daemon can resolve that project's
`.hum/config.yaml`. The daemon's own working directory cannot serve: under a
supervisor it is `$HOME`, so the client is the only process that knows where the
project is.

Validation of `root` is split. `Validate` rejects a relative path, which is a
pure check any receiver can make and a mistake no filesystem can excuse. Whether
the path *exists* is checked by the daemon when it resolves the config, because
only the daemon shares a filesystem with the project. A missing `root` is not an
error: it means global config only, which keeps the protocol usable from a bare
`socat` one-liner.

## Requests

A request carries **exactly one** of an event or a command. Both is ambiguous;
neither is a no-op the daemon would otherwise answer with a meaningless success.

The event form is **flat** — the event's own fields sit at the top level rather
than nested under a key, because the published shape does not follow Go's struct
layout:

```json
{"event":"session.started","id":"123","title":"build"}
```

The command form:

```json
{"command":"volume","value":"0.4"}
```

`value` is omitted when empty.

### Commands

| Command | `value` |
|---|---|
| `status` | — |
| `mute` / `unmute` | — |
| `volume` | fraction in `[0.0, 1.0]` |
| `theme.list` | — |
| `theme.use` | theme name |
| `audio.test` | — |
| `shutdown` | — |
| `ping` | — |

A command outside this set is rejected with `ErrUnknownCommand`.

`volume` is a fraction rather than decibels so the contract stays readable from a
shell script. `NaN` and the infinities are rejected: they parse as floats, and
every comparison against `NaN` is false, so a naive range check admits them.

### Validation is separate from decoding

Decoding a request does not validate it. A malformed message and a well-formed
one that breaks the contract are different failures and deserve different
answers, so callers invoke `Validate` themselves.

Encoding does refuse an invalid request, which keeps an ambiguous message off
the wire entirely.

## Responses

```json
{"ok":true,"data":{"sessions":2}}
{"ok":false,"error":"no such theme"}
```

`ok` is always present. `error` appears only on failure, `data` only when the
command returns a payload. `data` is carried raw, so the envelope needs no
knowledge of any command's payload shape.

### Command payloads

Payload shapes live in `internal/protocol` rather than in either binary, so the
daemon that writes them and the client that renders them cannot drift.

`status` returns the registry snapshot and the daemon's current musical context:

```json
{"ok":true,"data":{
  "sessions":[{"id":"a1","workspace":"tofu","title":"Validate PR #142","state":"active","pitch":"D3","updates":0,"seconds":12.4}],
  "theme":"minimal","root":"D3","scale":"minor_pentatonic","context_owner":"/Users/dev/projects/tofu",
  "renderer":"audio","renderer_requested":"audio","sample_rate":48000,"version":"0.1.0",
  "volume":0.6,"muted":false,"sounding_voices":1
}}
```

`context_owner` is the project whose configuration supplied the current root,
scale and theme, and is omitted when none did. `sounding_voices` counts sustained
drones, which is not the same as the number of sessions: a terminal session is
still listed until it is reaped.

A session's `pitch` is the note its drone is sounding, so an operator can
correlate what they hear with what is running. It is omitted once the voice is
released, which is what distinguishes a session that is still audible from one
that is merely still listed.

`renderer`, `renderer_requested`, `sample_rate` and `version` describe the daemon
rather than the work. `renderer` is what is running and `renderer_requested` is
what was asked for, so a client can tell a deliberate `humd --no-audio` from a
fallback to `nop` because no device was available — identical from `renderer`
alone, and opposite diagnoses. `sample_rate` is 0 for a renderer that cannot
report one, and `version` lets `hum doctor` compare the client and daemon builds
over the connection it already uses.

`theme.list` returns `{"themes":["minimal"],"active":"minimal"}` — the available
themes plus the one in force, so a client can mark it without a second request.
`theme.use` returns `{"theme":"minimal"}`, the theme it switched to, so a client
can confirm the switch rather than assume it.

`audio.test` plays a two-second tone and returns
`{"played":true,"renderer":"audio","muted":false,"seconds":2}`. `played` is
false when nothing could be heard — a `nop` renderer or a muted daemon — because
a diagnostic that reports success into silence is worse than no diagnostic.

`ping`, `mute`, `unmute`, `volume` and `shutdown` carry no payload.

## Talking to the daemon without a Go client

No Go client is required — the protocol is line-oriented JSON. Verified against a
running `humd`:

```
$ echo '{"command":"ping"}' | nc -U ~/.hum/humd.sock
{"ok":true}
```

Batching works down one connection, and each request gets its own response line:

```
$ printf '{"event":"session.started","id":"a1","title":"build"}\n{"event":"session.completed","id":"a1"}\n' | nc -U ~/.hum/humd.sock
{"ok":true}
{"ok":true}
```

`nc -U` is used above because macOS ships it. Where `nc` has no `-U`, `socat`
opens the same connection, and the same two examples were run through it against
a live daemon:

```
$ printf '{"command":"ping"}\n' | socat - UNIX-CONNECT:$HOME/.hum/humd.sock
{"ok":true}
$ printf '{"event":"session.started","id":"a1","title":"build"}\n{"event":"session.completed","id":"a1"}\n' | socat - UNIX-CONNECT:$HOME/.hum/humd.sock
{"ok":true}
{"ok":true}
```

Despite the name `socat` is a separate program, not a variant of `cat`: `cat`
only opens a path, and a Unix socket needs `socket` plus `connect`. Against a
live daemon `cat humd.sock` fails with `No such device or address` and
`cat > humd.sock` with `Operation not supported on socket`.
