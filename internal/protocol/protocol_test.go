package protocol

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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

	t.Run("session.started carries an absolute project root", func(t *testing.T) {
		const wire = `{"event":"session.started","id":"123","root":"/Users/dev/projects/tofu"}`

		var got Event
		if err := json.Unmarshal([]byte(wire), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Root != "/Users/dev/projects/tofu" {
			t.Errorf("decoded root = %q, want %q", got.Root, "/Users/dev/projects/tofu")
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

	t.Run("accepts an absolute project root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "project")
		if err := (Event{Event: SessionStarted, ID: "1", Root: root}).Validate(); err != nil {
			t.Errorf("Validate() with the absolute root %q = %v, want nil", root, err)
		}
	})

	t.Run("rejects a relative project root", func(t *testing.T) {
		err := Event{Event: SessionStarted, ID: "1", Root: "projects/tofu"}.Validate()
		if !errors.Is(err, ErrRelativeRoot) {
			t.Errorf("Validate() with a relative root = %v, want ErrRelativeRoot", err)
		}
	})

	t.Run("accepts an absent project root", func(t *testing.T) {
		if err := (Event{Event: SessionCompleted, ID: "1"}).Validate(); err != nil {
			t.Errorf("Validate() with no root = %v, want nil", err)
		}
	})
}

func TestEventIDLimitCountsBytesNotRunes(t *testing.T) {
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

func TestMaxIDLenIs128(t *testing.T) {
	if MaxIDLen != 128 {
		t.Errorf("MaxIDLen = %d, want 128: the maximum id length is specified as 128 bytes in the wire protocol", MaxIDLen)
	}
}

func TestOwnerPIDValidation(t *testing.T) {
	t.Run("accepts a positive owner_pid", func(t *testing.T) {
		ev := Event{Event: SessionStarted, ID: "1", OwnerPID: 12345, OwnerHost: "mymachine"}
		if err := ev.Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil for a positive owner_pid", err)
		}
	})

	t.Run("accepts zero owner_pid as unset", func(t *testing.T) {
		ev := Event{Event: SessionStarted, ID: "1"}
		if err := ev.Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil for zero owner_pid", err)
		}
	})

	t.Run("rejects a negative owner_pid", func(t *testing.T) {
		ev := Event{Event: SessionStarted, ID: "1", OwnerPID: -1}
		if err := ev.Validate(); err == nil {
			t.Error("Validate() = nil for negative owner_pid, want an error")
		}
	})
}

func TestOwnerFieldsRoundTrip(t *testing.T) {
	ev := Event{
		Event:     SessionStarted,
		ID:        "abc",
		OwnerPID:  9999,
		OwnerHost: "buildhost",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.OwnerPID != ev.OwnerPID {
		t.Errorf("OwnerPID = %d, want %d", got.OwnerPID, ev.OwnerPID)
	}
	if got.OwnerHost != ev.OwnerHost {
		t.Errorf("OwnerHost = %q, want %q", got.OwnerHost, ev.OwnerHost)
	}
}
