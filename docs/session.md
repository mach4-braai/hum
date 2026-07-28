# Session Package

## State Machine

| From state  | Event               | Result state | Error                  |
|-------------|---------------------|--------------|------------------------|
| active      | session.started     | —            | ErrDuplicateID         |
| active      | session.updated     | active       | —                      |
| active      | session.completed   | completed    | —                      |
| active      | session.failed      | failed       | —                      |
| active      | session.cancelled   | cancelled    | —                      |
| completed   | session.started     | active       | —  (restart)           |
| completed   | session.updated     | —            | ErrAlreadyTerminal     |
| completed   | session.completed   | —            | ErrAlreadyTerminal     |
| completed   | session.failed      | —            | ErrAlreadyTerminal     |
| completed   | session.cancelled   | —            | ErrAlreadyTerminal     |
| failed      | session.started     | active       | —  (restart)           |
| failed      | session.updated     | —            | ErrAlreadyTerminal     |
| failed      | session.completed   | —            | ErrAlreadyTerminal     |
| failed      | session.failed      | —            | ErrAlreadyTerminal     |
| failed      | session.cancelled   | —            | ErrAlreadyTerminal     |
| cancelled   | session.started     | active       | —  (restart)           |
| cancelled   | session.updated     | —            | ErrAlreadyTerminal     |
| cancelled   | session.completed   | —            | ErrAlreadyTerminal     |
| cancelled   | session.failed      | —            | ErrAlreadyTerminal     |
| cancelled   | session.cancelled   | —            | ErrAlreadyTerminal     |
| (unknown)   | session.updated     | —            | ErrUnknownSession      |
| (unknown)   | session.completed   | —            | ErrUnknownSession      |
| (unknown)   | session.failed      | —            | ErrUnknownSession      |
| (unknown)   | session.cancelled   | —            | ErrUnknownSession      |

## Restart Decision

A `session.started` event for an ID whose current session is terminal replaces the old session and resets it to active with a fresh `StartedAt` and zeroed `Updates`. The terminal record is not kept.

Rationale: a retried piece of work must be able to sound again. The old terminal state becomes irrelevant once the same logical unit of work is retried; retaining two records under the same ID would require callers to distinguish them, adding complexity that serves no use case in the daemon.

A `session.started` for an ID whose session is still active is rejected with `ErrDuplicateID` because two concurrent starts for the same ID are almost certainly a bug in the caller.

## Why Snapshot Deep-Copies

`Snapshot` returns a slice of independent `Session` values, each with its own copy of the `Metadata` map. Without this, callers mutating a returned session (including its metadata) would silently corrupt the registry's stored state. Deep copying on the way out keeps the registry's map as the single mutable owner of all session data.

The sort order — ascending `StartedAt`, then ascending `ID` on ties — is deterministic, which matters for `hum status` output and for reproducible test assertions.

## What Reap Is For

The daemon runs indefinitely. Terminal sessions accumulate in memory after their completion phrases have played and their entries have been displayed. `Reap` gives the daemon a way to evict terminal sessions older than a configurable age, bounding the registry's memory footprint. Active sessions are never reaped regardless of age; only sessions with a non-zero `EndedAt` that precedes the cutoff are removed.
