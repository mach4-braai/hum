// Package protocol defines Hum's wire contract: the generic work-session events
// clients emit and the framing they travel in.
//
// PRD.md section 22 makes this a stable public contract and section 3 requires
// it to stay provider agnostic. Nothing here may reference an AI tool, an agent
// framework, or any specific client: Hum understands work sessions and nothing
// else. A field that only one integration would set belongs in Metadata.
package protocol

import (
	"errors"
	"fmt"
)

// EventType identifies a work-session lifecycle transition. The set is closed:
// PRD.md section 14 defines exactly these five.
type EventType string

const (
	SessionStarted   EventType = "session.started"
	SessionUpdated   EventType = "session.updated"
	SessionCompleted EventType = "session.completed"
	SessionFailed    EventType = "session.failed"
	SessionCancelled EventType = "session.cancelled"
)

// Event is a single lifecycle message about one work session.
//
// Every field except Event and ID is omitempty, so a message carries only what
// the client actually knows. Clients that diff payloads or match on exact shape
// would otherwise see spurious empty keys appear.
type Event struct {
	Event     EventType         `json:"event"`
	ID        string            `json:"id"`
	Workspace string            `json:"workspace,omitempty"`
	Title     string            `json:"title,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// MaxIDLen bounds a session id in **bytes**, not runes. The id travels on every
// message and is retained per session, so a byte bound is what actually caps
// memory and wire size; a rune bound would let a multibyte id consume up to four
// times the intended budget.
const MaxIDLen = 128

// ErrUnknownEvent reports an event type outside the closed set in PRD.md
// section 14. Callers match it with errors.Is to distinguish a client using a
// newer protocol from a malformed message.
var ErrUnknownEvent = errors.New("unknown event type")

// Known reports whether t is one of the five documented event types.
func (t EventType) Known() bool {
	switch t {
	case SessionStarted, SessionUpdated, SessionCompleted, SessionFailed, SessionCancelled:
		return true
	}
	return false
}

// Validate checks an event against the wire contract.
//
// This is a trust boundary, not a convenience: the daemon accepts messages from
// any local process. An event admitted with an empty id becomes an unaddressable
// session, so nothing can ever complete it and its drone sustains forever.
func (e Event) Validate() error {
	if !e.Event.Known() {
		return fmt.Errorf("%w: %q", ErrUnknownEvent, e.Event)
	}
	if e.ID == "" {
		return errors.New("event id is required")
	}
	if len(e.ID) > MaxIDLen {
		return fmt.Errorf("event id is %d bytes, limit is %d", len(e.ID), MaxIDLen)
	}
	return nil
}

// ParseEventType converts a wire string to an EventType, rejecting anything
// outside the closed set. The comparison is exact: the wire values are literal
// strings, and accepting case variants or padded input would turn the closed set
// into a suggestion and let a typo look like a delivered event.
func ParseEventType(s string) (EventType, error) {
	t := EventType(s)
	if !t.Known() {
		return "", fmt.Errorf("%w: %q", ErrUnknownEvent, s)
	}
	return t, nil
}
