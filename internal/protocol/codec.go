package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxMessageLen bounds a single wire message in bytes, excluding the framing
// newline. A message of exactly this size is legal.
//
// This is a denial-of-service boundary, not a tidiness rule: without it any local
// process can drive the daemon's memory by opening the socket and streaming bytes
// without ever sending a newline.
const MaxMessageLen = 64 << 10

// ErrMessageTooLarge reports a wire message beyond MaxMessageLen.
var ErrMessageTooLarge = errors.New("message exceeds the maximum length")

// Decoder reads newline-delimited events from a stream.
//
// It owns a single buffered reader for the life of the connection. Constructing
// a fresh buffered reader per message would read ahead and discard whatever it
// had buffered beyond the first newline, silently losing the messages that
// followed on the same socket.
//
// Decode does not validate: framing and semantics are separate concerns, so
// callers apply Event.Validate at the trust boundary, where they can answer with
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
// Blank lines are skipped rather than reported: a client may write a bare newline
// as a keepalive, and treating that as malformed would drop an otherwise healthy
// connection.
//
// After ErrMessageTooLarge the reader is left at an unspecified position, because
// the remainder of the oversized line has not been consumed. Callers must close
// the connection rather than attempt to resynchronise; continuing would parse the
// tail of a rejected message as a new one.
func (d *Decoder) Decode() (Event, error) {
	for {
		line, err := d.readLine()
		if err != nil {
			return Event{}, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		// A message is a JSON object. This check is not redundant with
		// Unmarshal: `null` unmarshals into a struct without error and leaves
		// every field zero, so it would otherwise be reported as a successful
		// decode of an empty event, indistinguishable from a real message.
		if line[0] != '{' {
			return Event{}, fmt.Errorf("decode event: message must be a JSON object, got %.16q", line)
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return Event{}, fmt.Errorf("decode event: %w", err)
		}
		return e, nil
	}
}

// readLine returns one line without its trailing newline, refusing to accumulate
// more than MaxMessageLen bytes.
//
// bufio.Reader.ReadString and ReadBytes are unsuitable here: both grow their
// result until the delimiter arrives, so an unterminated line would be buffered
// without bound, which is exactly the case MaxMessageLen exists to stop. ReadSlice
// returns what fits in the fixed buffer and reports ErrBufferFull, letting the
// length be checked as the line accumulates.
func (d *Decoder) readLine() ([]byte, error) {
	var line []byte
	for {
		chunk, err := d.r.ReadSlice('\n')

		if errors.Is(err, bufio.ErrBufferFull) {
			if len(line)+len(chunk) > MaxMessageLen {
				return nil, fmt.Errorf("%w of %d bytes", ErrMessageTooLarge, MaxMessageLen)
			}
			line = append(line, chunk...)
			continue
		}

		if err != nil {
			if errors.Is(err, io.EOF) && len(line)+len(chunk) > 0 {
				// A final line with no trailing newline, which is what a
				// client using printf or a closing pipe produces.
				line = append(line, chunk...)
				if len(line) > MaxMessageLen {
					return nil, fmt.Errorf("%w of %d bytes", ErrMessageTooLarge, MaxMessageLen)
				}
				return line, nil
			}
			return nil, err
		}

		payload := chunk[:len(chunk)-1]
		if len(line)+len(payload) > MaxMessageLen {
			return nil, fmt.Errorf("%w of %d bytes", ErrMessageTooLarge, MaxMessageLen)
		}
		return append(line, payload...), nil
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
// containing a newline cannot desynchronise the framing of the messages that
// follow it.
//
// An oversized event is refused before anything is written: the daemon must not
// emit a message its own decoder would reject, and a partial line would
// desynchronise the stream for every message after it.
func (e *Encoder) Encode(ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if len(data) > MaxMessageLen {
		return fmt.Errorf("%w: event is %d bytes, limit is %d", ErrMessageTooLarge, len(data), MaxMessageLen)
	}
	if _, err := e.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
