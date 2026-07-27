// Package protocol defines Hum's wire contract: the generic work-session events
// clients emit and the framing they travel in.
//
// PRD.md section 22 makes this a stable public contract and section 3 requires
// it to stay provider agnostic. Nothing here may reference an AI tool, an agent
// framework, or any specific client: Hum understands work sessions and nothing
// else. A field that only one integration would set belongs in Metadata.
package protocol

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
