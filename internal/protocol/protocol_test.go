package protocol

import (
	"encoding/json"
	"testing"
)

// PRD.md section 22 calls the event protocol a stable public contract, and
// section 14 gives these exact payloads. Third-party clients will be written
// against them, so they are pinned literally: a field rename or an added
// mandatory field is a breaking change, not a refactor.
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
		if got != want {
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
