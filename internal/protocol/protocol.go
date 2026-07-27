// Package protocol defines Hum's wire contract: the generic work-session events
// clients emit and the framing they travel in.
//
// Nothing here may reference an AI tool, an agent framework or a specific client
// (PRD.md section 3). A field only one integration would set belongs in Metadata.
package protocol

import (
	"errors"
	"fmt"
)

// EventType identifies a work-session lifecycle transition. The set is closed.
type EventType string

const (
	SessionStarted   EventType = "session.started"
	SessionUpdated   EventType = "session.updated"
	SessionCompleted EventType = "session.completed"
	SessionFailed    EventType = "session.failed"
	SessionCancelled EventType = "session.cancelled"
)

// Event is a single lifecycle message about one work session. Every field except
// Event and ID is omitempty, so a message carries only what the client knows.
type Event struct {
	Event     EventType         `json:"event"`
	ID        string            `json:"id"`
	Workspace string            `json:"workspace,omitempty"`
	Title     string            `json:"title,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// MaxIDLen bounds a session id in bytes, not runes: bytes are what cap wire size
// and retained memory.
const MaxIDLen = 128

// ErrUnknownEvent reports an event type outside the closed set. Callers match it
// with errors.Is to tell a newer client from a malformed message.
var ErrUnknownEvent = errors.New("unknown event type")

// Known reports whether t is one of the five documented event types.
func (t EventType) Known() bool {
	switch t {
	case SessionStarted, SessionUpdated, SessionCompleted, SessionFailed, SessionCancelled:
		return true
	}
	return false
}

// Validate checks an event against the wire contract. It is a trust boundary: an
// empty id would become an unaddressable session whose drone never stops.
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

// ParseEventType converts a wire string to an EventType. The comparison is exact:
// accepting case variants or padding would make the closed set a suggestion.
func ParseEventType(s string) (EventType, error) {
	t := EventType(s)
	if !t.Known() {
		return "", fmt.Errorf("%w: %q", ErrUnknownEvent, s)
	}
	return t, nil
}
