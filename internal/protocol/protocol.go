// Package protocol defines Hum's wire contract: newline-delimited JSON for
// work-session events and daemon control. It is public, so shapes change
// additively or not at all, and nothing here may name an AI tool, agent
// framework, or client.
//
// The contract is specified in docs/protocol.md.
package protocol

import (
	"errors"
	"fmt"
)

type EventType string

const (
	SessionStarted   EventType = "session.started"
	SessionUpdated   EventType = "session.updated"
	SessionCompleted EventType = "session.completed"
	SessionFailed    EventType = "session.failed"
	SessionCancelled EventType = "session.cancelled"
)

type Event struct {
	Event     EventType         `json:"event"`
	ID        string            `json:"id"`
	Workspace string            `json:"workspace,omitempty"`
	Title     string            `json:"title,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

const MaxIDLen = 128

var ErrUnknownEvent = errors.New("unknown event type")

func (t EventType) Known() bool {
	switch t {
	case SessionStarted, SessionUpdated, SessionCompleted, SessionFailed, SessionCancelled:
		return true
	}
	return false
}

// Trust boundary: an empty id would become an unaddressable session whose drone
// never stops.
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

// The comparison is exact: accepting case variants or padding would make the
// closed set a suggestion.
func ParseEventType(s string) (EventType, error) {
	t := EventType(s)
	if !t.Known() {
		return "", fmt.Errorf("%w: %q", ErrUnknownEvent, s)
	}
	return t, nil
}
