package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxMessageLen = 64 << 10

var ErrMessageTooLarge = errors.New("message exceeds the maximum length")

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Decode() (Event, error) {
	for {
		line, err := d.readBoundedLine()
		if err != nil {
			return Event{}, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
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

func (d *Decoder) readBoundedLine() ([]byte, error) {
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

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

func (e *Encoder) Encode(ev Event) error {
	data, _ := json.Marshal(ev)
	if len(data) > MaxMessageLen {
		return fmt.Errorf("%w: event is %d bytes, limit is %d", ErrMessageTooLarge, len(data), MaxMessageLen)
	}
	if _, err := e.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
