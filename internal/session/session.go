package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

var now = time.Now

type State string

const (
	StateNone      State = ""
	StateActive    State = "active"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

var (
	ErrAlreadyTerminal   = errors.New("session is already terminal")
	ErrDuplicateID       = errors.New("session id already active")
	ErrUnknownSession    = errors.New("unknown session id")
	ErrInvalidTransition = errors.New("invalid transition")
)

type Session struct {
	ID        string
	Workspace string
	Title     string
	State     State
	Priority  int
	Metadata  map[string]string
	StartedAt time.Time
	EndedAt   time.Time
	Updates   int
}

func (s State) Terminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCancelled:
		return true
	}
	return false
}

func (s Session) Duration() time.Duration {
	if s.State.Terminal() {
		return s.EndedAt.Sub(s.StartedAt)
	}
	return now().Sub(s.StartedAt)
}

func Transition(from State, ev protocol.EventType) (State, error) {
	if from == StateNone {
		if ev == protocol.SessionStarted {
			return StateActive, nil
		}
		return "", fmt.Errorf("%w: on %q", ErrUnknownSession, ev)
	}
	if from.Terminal() {
		if ev == protocol.SessionStarted {
			return StateActive, nil
		}
		return "", fmt.Errorf("%w: from %q on %q", ErrAlreadyTerminal, from, ev)
	}
	switch ev {
	case protocol.SessionStarted:
		return "", fmt.Errorf("%w: from %q on %q", ErrDuplicateID, from, ev)
	case protocol.SessionUpdated:
		return StateActive, nil
	case protocol.SessionCompleted:
		return StateCompleted, nil
	case protocol.SessionFailed:
		return StateFailed, nil
	case protocol.SessionCancelled:
		return StateCancelled, nil
	}
	return "", fmt.Errorf("%w: from %q on %q", ErrInvalidTransition, from, ev)
}
