# Protocol

Hum's wire contract. It is public: third-party clients depend on it, so the
shapes here change additively or not at all. Nothing in the protocol names an AI
tool, agent framework or client.

**Implementation status.** The message types, framing and envelope described
here are implemented in `internal/protocol`, and the `hum` client speaks them.
The daemon does not yet listen: `humd` is still a usage stub, and the socket
server arrives with #23, #24 and #25. Per-command response payloads are
specified alongside the commands that produce them.

## Framing

Messages are single LF-terminated lines of JSON, bounded by `MaxMessageLen`
(64 KiB) excluding the newline.

A decoder that reports `ErrMessageTooLarge` has not consumed the rest of the
oversized line and cannot resynchronise, so the caller must close the connection
rather than keep reading.

Blank lines are skipped. A final line without a trailing newline is accepted, so
`printf` and a closing pipe both work.

## Transport

The socket path is `$HUM_SOCKET`, else `humd.sock` inside the config directory.
`hum` dials it, writes one request, reads one response and closes; it never
reuses a connection, so a wedged daemon cannot leave the client holding a
half-consumed stream. Whether the daemon will accept concurrent connections is
decided by #23.

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

## Talking to the daemon without a Go client

No Go client is required — the protocol is line-oriented JSON. Once the socket
server lands (#23), this will be the shape:

```
$ echo '{"command":"ping"}' | socat - UNIX-CONNECT:$HOME/.hum/humd.sock
{"ok":true}
```

**Not yet runnable:** `humd` does not listen, so this example is unverified and
#35 must confirm it against a running daemon before the docs claim otherwise.
