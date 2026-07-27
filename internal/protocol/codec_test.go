package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// One JSON object per line, so clients can stream and socat works unaided.
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

// eventLineOfLength builds an event whose JSON is exactly n bytes. ASCII padding
// needs no escaping, so length grows one byte per character.
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

// The limit is measured on the payload, excluding the framing LF.
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

// Decode must refuse malformed input and keep the syntax error reachable.
func TestDecoderRejectsMalformedJSON(t *testing.T) {
	t.Run("truncated object", func(t *testing.T) {
		_, err := NewDecoder(strings.NewReader(`{"event":"session.started","id":` + "\n")).Decode()
		if err == nil {
			t.Fatal("Decode() = nil, want an error for a truncated object")
		}
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Errorf("Decode() = %v, want a wrapped *json.SyntaxError so the failure position survives", err)
		}
	})

	// A JSON null unmarshals into a struct without error, leaving every field
	// zero. Without an explicit guard the decoder reports success and hands back
	// an empty event, which looks identical to a legitimately decoded message.
	t.Run("null is not a message", func(t *testing.T) {
		if _, err := NewDecoder(strings.NewReader("null\n")).Decode(); err == nil {
			t.Error("Decode() = nil for a null payload, want an error; a zero event must not look like a successful decode")
		}
	})

	for _, payload := range []string{"42", `"session.started"`, "[]", "true"} {
		t.Run("non-object payload "+payload, func(t *testing.T) {
			if _, err := NewDecoder(strings.NewReader(payload + "\n")).Decode(); err == nil {
				t.Errorf("Decode() = nil for %s, want an error: a message must be a JSON object", payload)
			}
		})
	}
}
