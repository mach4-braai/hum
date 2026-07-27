package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// Framing is one JSON object per line, so a client may hold a connection open
// and stream events, and so `socat`-style manual use works without a length
// prefix. PRD.md section 16 makes this the MVP transport contract.
func TestDecoderReadsAStreamOfEvents(t *testing.T) {
	stream := strings.Join([]string{
		`{"event":"session.started","id":"1","title":"build"}`,
		`{"event":"session.updated","id":"1"}`,
		`{"event":"session.completed","id":"1"}`,
	}, "\n") + "\n"

	dec := NewDecoder(strings.NewReader(stream))

	for _, want := range []EventType{SessionStarted, SessionUpdated, SessionCompleted} {
		got, err := dec.Decode()
		if err != nil {
			t.Fatalf("Decode() = error %v, want %s", err, want)
		}
		if got.Event != want {
			t.Errorf("Decode() event = %s, want %s", got.Event, want)
		}
		if got.ID != "1" {
			t.Errorf("Decode() id = %q, want %q", got.ID, "1")
		}
	}

	if _, err := dec.Decode(); !errors.Is(err, io.EOF) {
		t.Errorf("Decode() after the last event = %v, want io.EOF", err)
	}
}

// A final line without a trailing newline is what a client using `printf` or a
// closed pipe produces. Dropping it would lose the most common one-shot message.
func TestDecoderAcceptsFinalLineWithoutNewline(t *testing.T) {
	dec := NewDecoder(strings.NewReader(`{"event":"session.completed","id":"7"}`))

	got, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode() = %v, want an event", err)
	}
	if got.ID != "7" {
		t.Errorf("Decode() id = %q, want %q", got.ID, "7")
	}
	if _, err := dec.Decode(); !errors.Is(err, io.EOF) {
		t.Errorf("second Decode() = %v, want io.EOF", err)
	}
}

// Blank lines appear when a client writes "\n" as a keepalive or double-newlines
// by accident. Treating them as malformed would drop otherwise fine connections.
func TestDecoderSkipsBlankLines(t *testing.T) {
	dec := NewDecoder(strings.NewReader("\n\n" + `{"event":"session.failed","id":"9"}` + "\n\n"))

	got, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode() = %v, want an event", err)
	}
	if got.Event != SessionFailed {
		t.Errorf("Decode() event = %s, want %s", got.Event, SessionFailed)
	}
	if _, err := dec.Decode(); !errors.Is(err, io.EOF) {
		t.Errorf("second Decode() = %v, want io.EOF", err)
	}
}

// The encoder must emit exactly one line per event: an embedded newline would
// desynchronise the framing for every subsequent message on the connection.
func TestEncoderWritesOneLinePerEvent(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	events := []Event{
		{Event: SessionStarted, ID: "1", Title: "a\nb", Metadata: map[string]string{"k": "v\nw"}},
		{Event: SessionCompleted, ID: "1"},
	}
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("Encode(%s) = %v", e.Event, err)
		}
	}

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output %q does not end with a newline", out)
	}
	if got := strings.Count(out, "\n"); got != len(events) {
		t.Errorf("output has %d newlines, want %d; embedded newlines must be escaped", got, len(events))
	}

	// What the encoder writes must be readable by the decoder.
	dec := NewDecoder(strings.NewReader(out))
	first, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode of encoded output = %v", err)
	}
	if first.Title != "a\nb" {
		t.Errorf("round-tripped title = %q, want %q", first.Title, "a\nb")
	}
}

// eventLineOfLength builds a valid event whose JSON encoding is exactly n bytes,
// padding the title. The padding is plain ASCII so it needs no escaping and the
// length grows one byte per character.
func eventLineOfLength(t *testing.T, n int) string {
	t.Helper()
	probe, err := json.Marshal(Event{Event: SessionStarted, ID: "1", Title: "x"})
	if err != nil {
		t.Fatalf("probe marshal: %v", err)
	}
	overhead := len(probe) - 1
	pad := n - overhead
	if pad < 0 {
		t.Fatalf("cannot build a %d byte event; overhead alone is %d", n, overhead)
	}
	out, err := json.Marshal(Event{Event: SessionStarted, ID: "1", Title: strings.Repeat("x", pad)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(out) != n {
		t.Fatalf("built a %d byte event, want %d", len(out), n)
	}
	return string(out)
}

// An unbounded line length lets any local process drive the daemon's memory by
// opening a socket and never sending a newline. The limit is measured on the
// payload, excluding the framing LF, so that a message of exactly the documented
// size is legal rather than off-by-one rejected.
func TestDecoderEnforcesMessageSizeLimit(t *testing.T) {
	t.Run("accepts a payload of exactly the limit", func(t *testing.T) {
		line := eventLineOfLength(t, MaxMessageLen)

		got, err := NewDecoder(strings.NewReader(line + "\n")).Decode()
		if err != nil {
			t.Fatalf("Decode() of a %d byte payload = %v, want it accepted", MaxMessageLen, err)
		}
		if got.ID != "1" {
			t.Errorf("Decode() id = %q, want %q", got.ID, "1")
		}
	})

	t.Run("rejects a payload one byte over the limit", func(t *testing.T) {
		line := eventLineOfLength(t, MaxMessageLen+1)

		_, err := NewDecoder(strings.NewReader(line + "\n")).Decode()
		if !errors.Is(err, ErrMessageTooLarge) {
			t.Errorf("Decode() of a %d byte payload = %v, want ErrMessageTooLarge", MaxMessageLen+1, err)
		}
	})

	// A client that opens a connection and streams bytes without ever sending a
	// newline must be rejected on the limit, not buffered indefinitely.
	t.Run("rejects an unterminated oversized line", func(t *testing.T) {
		_, err := NewDecoder(strings.NewReader(strings.Repeat("x", MaxMessageLen+1))).Decode()
		if !errors.Is(err, ErrMessageTooLarge) {
			t.Errorf("Decode() of an unterminated oversized line = %v, want ErrMessageTooLarge", err)
		}
	})
}

// The daemon must not emit a message its own decoder would refuse, which would
// be a protocol violation visible only to the client.
func TestEncoderRefusesOversizedEvents(t *testing.T) {
	oversized := Event{Event: SessionStarted, ID: "1", Title: strings.Repeat("x", MaxMessageLen)}

	var buf bytes.Buffer
	err := NewEncoder(&buf).Encode(oversized)

	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("Encode() = %v, want ErrMessageTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %d bytes on a rejected event, want none: a partial line desynchronises the stream", buf.Len())
	}
}
