package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decoder reads newline-delimited events from a stream.
//
// It owns a single buffered reader for the life of the connection. Constructing
// a fresh buffered reader per message would read ahead and discard whatever it
// had buffered beyond the first newline, silently losing the messages that
// followed on the same socket.
//
// Decode does not validate: framing and semantics are separate concerns, so
// callers apply Event.Validate at the trust boundary where they can answer with
// a protocol error.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Decode returns the next event, or io.EOF once the stream is exhausted.
//
// Blank lines are skipped rather than reported: a client may write a bare
// newline as a keepalive, and treating that as malformed would drop an otherwise
// healthy connection.
func (d *Decoder) Decode() (Event, error) {
	for {
		line, err := d.r.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return Event{}, err
		}
		if len(bytes.TrimSpace([]byte(line))) == 0 {
			if err == io.EOF {
				return Event{}, io.EOF
			}
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return Event{}, fmt.Errorf("decode event: %w", err)
		}
		return e, nil
	}
}

// Encoder writes newline-delimited events to a stream.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns an Encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes one event as a single LF-terminated line.
//
// encoding/json escapes newlines inside strings, so a title or metadata value
// containing a newline cannot desynchronise the framing for the messages that
// follow it.
func (e *Encoder) Encode(ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if _, err := e.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
