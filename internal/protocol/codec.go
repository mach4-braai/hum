package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxMessageLen bounds one wire message in bytes, excluding the framing newline.
// Unbounded lines would let a local process exhaust memory by withholding one.
const MaxMessageLen = 64 << 10

// ErrMessageTooLarge reports a wire message beyond MaxMessageLen.
var ErrMessageTooLarge = errors.New("message exceeds the maximum length")

// Decoder reads newline-delimited events. It owns one buffered reader for the
// connection's lifetime; a per-message reader would drop what it buffered past the
// first newline. Decode does not validate: callers apply Event.Validate.
type Decoder struct {
	r *bufio.Reader
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Decode returns the next event, or io.EOF once the stream is exhausted. Blank
// lines are skipped so a bare-newline keepalive does not drop the connection.
//
// After ErrMessageTooLarge the read position is unspecified, because the rest of
// the oversized line is unconsumed. Callers must close rather than resynchronise.
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
		// Not redundant with Unmarshal: `null` unmarshals into a struct without
		// error, so it would decode as a zero-valued Event.
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
// ReadString and ReadBytes grow until the delimiter, so an unterminated line
// buffers without bound. ReadSlice lets length be checked as the line grows.
func (d *Decoder) readLine() ([]byte, error) {
	var line []byte
	for {
		chunk, err := d.r.ReadSlice('\n')

		if errors.Is(err, bufio.ErrBufferFull) {
			if len(line)+len(chunk) > MaxMessageLen {
				return nil, tooLarge()
			}
			line = append(line, chunk...)
			continue
		}

		if err != nil {
			// A final line with no trailing newline, as printf or a closing
			// pipe produces.
			if errors.Is(err, io.EOF) && len(line)+len(chunk) > 0 {
				line = append(line, chunk...)
				if len(line) > MaxMessageLen {
					return nil, tooLarge()
				}
				return line, nil
			}
			return nil, err
		}

		payload := chunk[:len(chunk)-1]
		if len(line)+len(payload) > MaxMessageLen {
			return nil, tooLarge()
		}
		return append(line, payload...), nil
	}
}

func tooLarge() error {
	return fmt.Errorf("%w of %d bytes", ErrMessageTooLarge, MaxMessageLen)
}

// Encoder writes newline-delimited events to a stream.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns an Encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes one event as a single LF-terminated line. encoding/json escapes
// newlines in strings, so field content cannot desynchronise the framing.
//
// An oversized event is refused before any bytes are written: a partial line
// would desynchronise every message after it.
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
