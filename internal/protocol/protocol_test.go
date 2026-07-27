package protocol

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// PRD.md section 14's exact payloads. Third-party clients are written against
// these, so a rename or a newly mandatory field is a breaking change.
func TestPRDWireExamplesRoundTrip(t *testing.T) {
	t.Run("session.started with all documented fields", func(t *testing.T) {
		const wire = `{"event":"session.started","id":"123","workspace":"tofu","title":"Validate PR #142"}`

		var got Event
		if err := json.Unmarshal([]byte(wire), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}

		want := Event{
			Event:     SessionStarted,
			ID:        "123",
			Workspace: "tofu",
			Title:     "Validate PR #142",
		}
		// Compared with DeepEqual rather than ==: Event carries a Metadata
		// map, so the struct is not comparable and == would not compile.
		if !reflect.DeepEqual(got, want) {
			t.Errorf("decoded = %+v, want %+v", got, want)
		}

		out, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if string(out) != wire {
			t.Errorf("re-encoded = %s, want %s", out, wire)
		}
	})

	// The completion example carries only an id. Re-encoding must not introduce
	// empty workspace, title or priority keys: clients that diff payloads or
	// match on exact shape would see spurious changes.
	t.Run("session.completed omits absent fields", func(t *testing.T) {
		const wire = `{"event":"session.completed","id":"123"}`

		var got Event
		if err := json.Unmarshal([]byte(wire), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}

		out, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if string(out) != wire {
			t.Errorf("re-encoded = %s, want %s", out, wire)
		}
	})
}

// Validation is a trust boundary: an empty id becomes an unaddressable session
// whose drone never stops.
func TestEventValidation(t *testing.T) {
	valid := Event{Event: SessionStarted, ID: "123"}
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate() on a minimal valid event = %v, want nil", err)
	}

	for _, e := range []Event{
		{Event: SessionUpdated, ID: "a"},
		{Event: SessionCompleted, ID: "a"},
		{Event: SessionFailed, ID: "a"},
		{Event: SessionCancelled, ID: "a"},
	} {
		if err := e.Validate(); err != nil {
			t.Errorf("Validate() on %s = %v, want nil", e.Event, err)
		}
	}

	t.Run("rejects an unknown event type", func(t *testing.T) {
		err := Event{Event: "session.exploded", ID: "1"}.Validate()
		if !errors.Is(err, ErrUnknownEvent) {
			t.Errorf("Validate() = %v, want it to wrap ErrUnknownEvent", err)
		}
	})

	t.Run("rejects an empty event type", func(t *testing.T) {
		if err := (Event{ID: "1"}).Validate(); err == nil {
			t.Error("Validate() = nil, want an error for a missing event type")
		}
	})

	t.Run("rejects a missing id", func(t *testing.T) {
		if err := (Event{Event: SessionStarted}).Validate(); err == nil {
			t.Error("Validate() = nil, want an error for a missing id")
		}
	})

	// An unbounded id would be echoed back by `hum status` and stored per
	// session, so it is a cheap memory-amplification vector from any local
	// process.
	t.Run("rejects an id longer than the documented limit", func(t *testing.T) {
		if err := (Event{Event: SessionStarted, ID: strings.Repeat("x", MaxIDLen+1)}).Validate(); err == nil {
			t.Errorf("Validate() = nil, want an error for an id of %d bytes", MaxIDLen+1)
		}
	})

	t.Run("accepts an id at exactly the documented limit", func(t *testing.T) {
		if err := (Event{Event: SessionStarted, ID: strings.Repeat("x", MaxIDLen)}).Validate(); err != nil {
			t.Errorf("Validate() on an id of exactly %d bytes = %v, want nil", MaxIDLen, err)
		}
	})
}

// A byte limit, not a rune limit: multibyte ids would otherwise get 4x the budget.
func TestEventIDLimitCountsBytesNotRunes(t *testing.T) {
	// 65 two-byte runes: only 65 characters, but 130 bytes.
	id := strings.Repeat("é", MaxIDLen/2+1)
	if len([]rune(id)) > MaxIDLen {
		t.Fatalf("test setup: %d runes is already over the limit, so it cannot distinguish bytes from runes", len([]rune(id)))
	}
	if len(id) <= MaxIDLen {
		t.Fatalf("test setup: %d bytes does not exceed the limit", len(id))
	}

	if err := (Event{Event: SessionStarted, ID: id}).Validate(); err == nil {
		t.Errorf("Validate() = nil for an id of %d bytes (%d runes), want an error", len(id), len([]rune(id)))
	}
}

// Unknown input must be rejected, not silently become a type the daemon discards.
func TestParseEventType(t *testing.T) {
	for _, want := range []EventType{SessionStarted, SessionUpdated, SessionCompleted, SessionFailed, SessionCancelled} {
		got, err := ParseEventType(string(want))
		if err != nil {
			t.Errorf("ParseEventType(%q) = error %v, want %s", want, err, want)
			continue
		}
		if got != want {
			t.Errorf("ParseEventType(%q) = %s, want %s", want, got, want)
		}
	}

	for _, bad := range []string{"", "session.exploded", "SESSION.STARTED", "started", "session.started "} {
		if _, err := ParseEventType(bad); !errors.Is(err, ErrUnknownEvent) {
			t.Errorf("ParseEventType(%q) = %v, want it to wrap ErrUnknownEvent", bad, err)
		}
	}
}
