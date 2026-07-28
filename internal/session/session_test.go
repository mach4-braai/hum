package session

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestStateTerminal(t *testing.T) {
	cases := []struct {
		state    State
		terminal bool
	}{
		{StateActive, false},
		{StateCompleted, true},
		{StateFailed, true},
		{StateCancelled, true},
	}
	for _, c := range cases {
		if got := c.state.Terminal(); got != c.terminal {
			t.Errorf("State(%q).Terminal() = %v, want %v", c.state, got, c.terminal)
		}
	}
}

func TestTransition(t *testing.T) {
	type outcome struct {
		want    State
		wantErr error
	}
	states := []State{StateNone, StateActive, StateCompleted, StateFailed, StateCancelled}
	events := []protocol.EventType{
		protocol.SessionStarted,
		protocol.SessionUpdated,
		protocol.SessionCompleted,
		protocol.SessionFailed,
		protocol.SessionCancelled,
	}
	expected := map[State]map[protocol.EventType]outcome{
		StateNone: {
			protocol.SessionStarted:   {StateActive, nil},
			protocol.SessionUpdated:   {"", ErrUnknownSession},
			protocol.SessionCompleted: {"", ErrUnknownSession},
			protocol.SessionFailed:    {"", ErrUnknownSession},
			protocol.SessionCancelled: {"", ErrUnknownSession},
		},
		StateActive: {
			protocol.SessionStarted:   {"", ErrDuplicateID},
			protocol.SessionUpdated:   {StateActive, nil},
			protocol.SessionCompleted: {StateCompleted, nil},
			protocol.SessionFailed:    {StateFailed, nil},
			protocol.SessionCancelled: {StateCancelled, nil},
		},
		StateCompleted: {
			protocol.SessionStarted:   {StateActive, nil},
			protocol.SessionUpdated:   {"", ErrAlreadyTerminal},
			protocol.SessionCompleted: {"", ErrAlreadyTerminal},
			protocol.SessionFailed:    {"", ErrAlreadyTerminal},
			protocol.SessionCancelled: {"", ErrAlreadyTerminal},
		},
		StateFailed: {
			protocol.SessionStarted:   {StateActive, nil},
			protocol.SessionUpdated:   {"", ErrAlreadyTerminal},
			protocol.SessionCompleted: {"", ErrAlreadyTerminal},
			protocol.SessionFailed:    {"", ErrAlreadyTerminal},
			protocol.SessionCancelled: {"", ErrAlreadyTerminal},
		},
		StateCancelled: {
			protocol.SessionStarted:   {StateActive, nil},
			protocol.SessionUpdated:   {"", ErrAlreadyTerminal},
			protocol.SessionCompleted: {"", ErrAlreadyTerminal},
			protocol.SessionFailed:    {"", ErrAlreadyTerminal},
			protocol.SessionCancelled: {"", ErrAlreadyTerminal},
		},
	}

	for _, from := range states {
		for _, ev := range events {
			want, ok := expected[from][ev]
			if !ok {
				t.Fatalf("test table has no expectation for (%q, %q); every state and event pair must be covered", from, ev)
			}
			t.Run(fmt.Sprintf("%s+%s", from, ev), func(t *testing.T) {
				got, err := Transition(from, ev)
				if want.wantErr != nil {
					if !errors.Is(err, want.wantErr) {
						t.Fatalf("Transition(%q, %q) error = %v, want %v", from, ev, err, want.wantErr)
					}
					if got != "" {
						t.Errorf("Transition(%q, %q) returned state %q alongside an error, want the zero state", from, ev, got)
					}
					return
				}
				if err != nil {
					t.Fatalf("Transition(%q, %q) unexpected error: %v", from, ev, err)
				}
				if got != want.want {
					t.Errorf("Transition(%q, %q) = %q, want %q", from, ev, got, want.want)
				}
			})
		}
	}

	for _, from := range states {
		t.Run(fmt.Sprintf("%s+unknown event", from), func(t *testing.T) {
			_, err := Transition(from, protocol.EventType("session.exploded"))
			if err == nil {
				t.Fatalf("Transition(%q, %q) = nil error, want a rejection", from, "session.exploded")
			}
		})
	}
}

func TestSessionDuration(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ended := base.Add(5 * time.Second)
	later := base.Add(7 * time.Second)

	t.Run("terminal session uses EndedAt", func(t *testing.T) {
		s := Session{
			State:     StateCompleted,
			StartedAt: base,
			EndedAt:   ended,
		}
		old := now
		t.Cleanup(func() { now = old })
		now = func() time.Time { return later }
		got := s.Duration()
		if got != 5*time.Second {
			t.Errorf("Duration() = %v, want 5s", got)
		}
	})

	t.Run("active session uses now()", func(t *testing.T) {
		s := Session{
			State:     StateActive,
			StartedAt: base,
		}
		old := now
		t.Cleanup(func() { now = old })
		now = func() time.Time { return later }
		got := s.Duration()
		if got != 7*time.Second {
			t.Errorf("Duration() = %v, want 7s", got)
		}
	})
}
