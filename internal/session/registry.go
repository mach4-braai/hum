package session

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"
	ChangeUpdated ChangeKind = "updated"
	ChangeEnded   ChangeKind = "ended"
)

type Change struct {
	Kind    ChangeKind
	Session Session
	Prev    State
}

type Registry struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func New() *Registry {
	return &Registry{sessions: make(map[string]Session)}
}

func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (r *Registry) Apply(ev protocol.Event) (Change, error) {
	if err := ev.Validate(); err != nil {
		return Change{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing := r.sessions[ev.ID]
	prev := existing.State
	next, err := Transition(prev, ev.Event)
	if err != nil {
		return Change{}, fmt.Errorf("session %q: %w", ev.ID, err)
	}

	switch ev.Event {
	case protocol.SessionStarted:
		started := Session{
			ID:        ev.ID,
			Workspace: ev.Workspace,
			Title:     ev.Title,
			Priority:  ev.Priority,
			State:     next,
			Metadata:  copyMetadata(ev.Metadata),
			StartedAt: now(),
		}
		r.sessions[ev.ID] = started
		return Change{Kind: ChangeAdded, Session: copySession(started), Prev: prev}, nil

	case protocol.SessionUpdated:
		existing.State = next
		if ev.Title != "" {
			existing.Title = ev.Title
		}
		if ev.Workspace != "" {
			existing.Workspace = ev.Workspace
		}
		if existing.Metadata == nil && len(ev.Metadata) > 0 {
			existing.Metadata = make(map[string]string, len(ev.Metadata))
		}
		for k, v := range ev.Metadata {
			existing.Metadata[k] = v
		}
		existing.Updates++
		r.sessions[ev.ID] = existing
		return Change{Kind: ChangeUpdated, Session: copySession(existing), Prev: prev}, nil

	default:
		existing.State = next
		existing.EndedAt = now()
		r.sessions[ev.ID] = existing
		return Change{Kind: ChangeEnded, Session: copySession(existing), Prev: prev}, nil
	}
}

func (r *Registry) Snapshot() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, copySession(s))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

func (r *Registry) Reap(olderThan time.Duration) int {
	cutoff := now().Add(-olderThan)
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int
	for id, s := range r.sessions {
		if s.State.Terminal() && s.EndedAt.Before(cutoff) {
			delete(r.sessions, id)
			count++
		}
	}
	return count
}

func copySession(s Session) Session {
	s.Metadata = copyMetadata(s.Metadata)
	return s
}
