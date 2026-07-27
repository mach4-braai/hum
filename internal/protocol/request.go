package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

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

var ErrUnknownCommand = errors.New("unknown command")

const (
	MinVolume = 0.0
	MaxVolume = 1.0
)

func (c Command) Known() bool {
	switch c {
	case CmdStatus, CmdMute, CmdUnmute, CmdVolume, CmdThemeList, CmdThemeUse, CmdShutdown, CmdPing:
		return true
	}
	return false
}

type Request struct {
	Event   *Event
	Command Command
	Value   string
}

type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

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
	if !(volume >= MinVolume && volume <= MaxVolume) {
		return fmt.Errorf("volume %v is outside [%v, %v]", volume, MinVolume, MaxVolume)
	}
	return nil
}
