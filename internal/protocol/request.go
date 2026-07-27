package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Command identifies a daemon control operation. Control is deliberately kept
// out of EventType: a client that only reports sessions never sends these, and
// the published event schema must not grow daemon plumbing.
type Command string

const (
	CmdStatus    Command = "status"
	CmdMute      Command = "mute"
	CmdUnmute    Command = "unmute"
	CmdVolume    Command = "volume"
	CmdThemeList Command = "theme.list"
	CmdThemeUse  Command = "theme.use"
	CmdShutdown  Command = "shutdown"
	CmdPing      Command = "ping"
)

// ErrUnknownCommand reports a command outside the closed set, so a caller can
// tell a newer client from a malformed message.
var ErrUnknownCommand = errors.New("unknown command")

// Volume bounds for CmdVolume, expressed as a fraction rather than decibels:
// the wire contract stays readable to a shell script.
const (
	MinVolume = 0.0
	MaxVolume = 1.0
)

// Known reports whether c is one of the eight documented commands.
func (c Command) Known() bool {
	switch c {
	case CmdStatus, CmdMute, CmdUnmute, CmdVolume, CmdThemeList, CmdThemeUse, CmdShutdown, CmdPing:
		return true
	}
	return false
}

// Request is one message from a client, carrying exactly one of an event or a
// command.
type Request struct {
	Event   *Event
	Command Command
	Value   string
}

// Response is the daemon's reply. Data stays raw so the envelope does not need
// to know each command's payload shape.
type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// MarshalJSON keeps an event request in the flat published form instead of
// nesting it under a field: the wire contract must not follow Go's struct.
// An invalid request is refused rather than serialised into an ambiguous
// message that the daemon would have to guess at.
func (r Request) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	if r.Event != nil {
		return json.Marshal(*r.Event)
	}
	return json.Marshal(struct {
		Command Command `json:"command"`
		Value   string  `json:"value,omitempty"`
	}{r.Command, r.Value})
}

// UnmarshalJSON decodes both forms. It deliberately does not validate, so a
// caller can tell a malformed message from a well-formed one that breaks the
// contract, and answer each with a different error.
func (r *Request) UnmarshalJSON(data []byte) error {
	var probe struct {
		Event   *EventType `json:"event"`
		Command Command    `json:"command"`
		Value   string     `json:"value"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	*r = Request{Command: probe.Command, Value: probe.Value}
	if probe.Event == nil {
		return nil
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("decode request event: %w", err)
	}
	r.Event = &event
	return nil
}

// Validate enforces exactly one of event or command. It is a trust boundary:
// a request carrying both is ambiguous, and one carrying neither is a no-op the
// daemon would otherwise answer with a meaningless success.
func (r Request) Validate() error {
	switch {
	case r.Event != nil && r.Command != "":
		return errors.New("request carries both an event and a command")
	case r.Event == nil && r.Command == "":
		return errors.New("request carries neither an event nor a command")
	case r.Event != nil:
		return r.Event.Validate()
	}

	if !r.Command.Known() {
		return fmt.Errorf("%w: %q", ErrUnknownCommand, r.Command)
	}
	if r.Command != CmdVolume {
		return nil
	}
	volume, err := strconv.ParseFloat(r.Value, 64)
	if err != nil {
		return fmt.Errorf("volume %q is not a number", r.Value)
	}
	// Negated rather than `volume < Min || volume > Max`: every comparison
	// against NaN is false, so the direct form would admit it.
	if !(volume >= MinVolume && volume <= MaxVolume) {
		return fmt.Errorf("volume %v is outside [%v, %v]", volume, MinVolume, MaxVolume)
	}
	return nil
}
