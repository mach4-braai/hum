package protocol

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRequestDecodesTheFlatEventForm(t *testing.T) {
	var got Request
	if err := json.Unmarshal([]byte(`{"event":"session.started","id":"123"}`), &got); err != nil {
		t.Fatalf("Unmarshal = %v, want an event request", err)
	}

	if got.Event == nil {
		t.Fatal("event request decoded with no event")
	}
	if got.Event.Event != SessionStarted || got.Event.ID != "123" {
		t.Errorf("decoded event = %+v, want session.started with id 123", *got.Event)
	}
	if got.Command != "" {
		t.Errorf("decoded command = %q, want empty for an event request", got.Command)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestRequestDecodesTheCommandForm(t *testing.T) {
	var got Request
	if err := json.Unmarshal([]byte(`{"command":"volume","value":"0.4"}`), &got); err != nil {
		t.Fatalf("Unmarshal = %v, want a command request", err)
	}

	if got.Event != nil {
		t.Errorf("command request decoded with an event: %+v", *got.Event)
	}
	if got.Command != CmdVolume {
		t.Errorf("decoded command = %q, want %q", got.Command, CmdVolume)
	}
	if got.Value != "0.4" {
		t.Errorf("decoded value = %q, want %q", got.Value, "0.4")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestRequestDecodingDoesNotValidate(t *testing.T) {
	var got Request
	if err := json.Unmarshal([]byte(`{"event":"session.started","id":"1","command":"status"}`), &got); err != nil {
		t.Fatalf("Unmarshal = %v, want the ambiguous request to decode", err)
	}

	if err := got.Validate(); err == nil {
		t.Fatal("Validate() = nil for a request carrying both an event and a command")
	}
}

func TestRequestRejectsMalformedJSON(t *testing.T) {
	t.Run("not an object", func(t *testing.T) {
		var got Request
		if err := json.Unmarshal([]byte(`42`), &got); err == nil {
			t.Error("Unmarshal = nil for a non-object request")
		}
	})

	t.Run("event is not a string", func(t *testing.T) {
		var got Request
		if err := json.Unmarshal([]byte(`{"event":{"nested":true}}`), &got); err == nil {
			t.Error("Unmarshal = nil for a non-string event")
		}
	})

	t.Run("event field types disagree", func(t *testing.T) {
		var got Request
		if err := json.Unmarshal([]byte(`{"event":"session.started","priority":"high"}`), &got); err == nil {
			t.Error("Unmarshal = nil for a non-numeric priority")
		}
	})
}

func TestRequestValidateRejectsAnEmptyEnvelope(t *testing.T) {
	err := (Request{}).Validate()
	if err == nil {
		t.Fatal("Validate() = nil for a request carrying neither an event nor a command")
	}
	if errors.Is(err, ErrUnknownCommand) {
		t.Errorf("Validate() = %v, want a distinct empty-envelope error, not ErrUnknownCommand", err)
	}
}

func TestRequestValidatePropagatesEventFailures(t *testing.T) {
	req := Request{Event: &Event{Event: SessionStarted}}

	err := req.Validate()

	if err == nil {
		t.Fatal("Validate() = nil for an event with no id")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("Validate() = %v, want the event's own error about the missing id", err)
	}
}

func TestCommandKnownCoversTheClosedSet(t *testing.T) {
	for _, cmd := range []Command{
		CmdStatus, CmdMute, CmdUnmute, CmdVolume,
		CmdThemeList, CmdThemeUse, CmdShutdown, CmdPing,
	} {
		if !cmd.Known() {
			t.Errorf("Known() = false for the documented command %q", cmd)
		}
	}

	for _, cmd := range []Command{"", "Status", "status ", "restart"} {
		if cmd.Known() {
			t.Errorf("Known() = true for %q, want the set to stay closed", cmd)
		}
	}
}

func TestRequestValidateRejectsUnknownCommands(t *testing.T) {
	err := Request{Command: "restart"}.Validate()

	if !errors.Is(err, ErrUnknownCommand) {
		t.Errorf("Validate() = %v, want ErrUnknownCommand", err)
	}
}

func TestRequestValidateBoundsVolume(t *testing.T) {
	for _, value := range []string{"0", "0.4", "1", "1.0"} {
		if err := (Request{Command: CmdVolume, Value: value}).Validate(); err != nil {
			t.Errorf("Validate() with volume %q = %v, want nil", value, err)
		}
	}

	for _, value := range []string{"", "loud", "-0.1", "1.1", "2", "NaN", "nan", "Inf", "+Inf", "-Inf"} {
		if err := (Request{Command: CmdVolume, Value: value}).Validate(); err == nil {
			t.Errorf("Validate() with volume %q = nil, want it rejected", value)
		}
	}
}

func TestRequestValidateIgnoresValueOnOtherCommands(t *testing.T) {
	if err := (Request{Command: CmdThemeUse, Value: "minimal"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want theme.use to accept a value", err)
	}
	if err := (Request{Command: CmdPing, Value: "ignored"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want ping to tolerate a stray value", err)
	}
}

func TestRequestEncodesAnEventFlat(t *testing.T) {
	req := Request{Event: &Event{Event: SessionStarted, ID: "123"}}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal = %v, want the event form", err)
	}

	if got := string(data); got != `{"event":"session.started","id":"123"}` {
		t.Errorf("Marshal = %s, want the flat published form", got)
	}
}

func TestRequestEncodesACommand(t *testing.T) {
	t.Run("with a value", func(t *testing.T) {
		data, err := json.Marshal(Request{Command: CmdVolume, Value: "0.4"})
		if err != nil {
			t.Fatalf("Marshal = %v, want the command form", err)
		}
		if got := string(data); got != `{"command":"volume","value":"0.4"}` {
			t.Errorf("Marshal = %s, want the command form", got)
		}
	})

	t.Run("without a value", func(t *testing.T) {
		data, err := json.Marshal(Request{Command: CmdPing})
		if err != nil {
			t.Fatalf("Marshal = %v, want the command form", err)
		}
		if got := string(data); got != `{"command":"ping"}` {
			t.Errorf("Marshal = %s, want no value field", got)
		}
	})
}

func TestRequestRefusesToEncodeAnInvalidEnvelope(t *testing.T) {
	both := Request{Event: &Event{Event: SessionStarted, ID: "1"}, Command: CmdStatus}

	if _, err := json.Marshal(both); err == nil {
		t.Error("Marshal = nil error for a request carrying both an event and a command")
	}
	if _, err := json.Marshal(Request{}); err == nil {
		t.Error("Marshal = nil error for an empty request")
	}
}

func TestRequestRoundTrips(t *testing.T) {
	for _, want := range []Request{
		{Event: &Event{Event: SessionCompleted, ID: "7", Title: "build", Priority: 3}},
		{Command: CmdThemeUse, Value: "minimal"},
		{Command: CmdStatus},
	} {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%+v) = %v", want, err)
		}
		var got Request
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s) = %v", data, err)
		}

		if want.Event != nil {
			if got.Event == nil || !reflect.DeepEqual(*got.Event, *want.Event) {
				t.Errorf("round trip of %s lost the event: %+v", data, got)
			}
			continue
		}
		if got.Command != want.Command || got.Value != want.Value {
			t.Errorf("round trip of %s = %+v, want %+v", data, got, want)
		}
	}
}

func TestResponseOmitsEmptyFields(t *testing.T) {
	data, err := json.Marshal(Response{OK: true})
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}

	if got := string(data); got != `{"ok":true}` {
		t.Errorf("Marshal = %s, want a bare success envelope", got)
	}
}

func TestResponseCarriesRawData(t *testing.T) {
	data, err := json.Marshal(Response{OK: true, Data: json.RawMessage(`{"sessions":2}`)})
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}

	if got := string(data); got != `{"ok":true,"data":{"sessions":2}}` {
		t.Errorf("Marshal = %s, want the payload embedded rather than re-encoded as a string", got)
	}
}

func TestResponseReportsFailure(t *testing.T) {
	data, err := json.Marshal(Response{Error: "no such theme"})
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}

	if got := string(data); got != `{"ok":false,"error":"no such theme"}` {
		t.Errorf("Marshal = %s, want ok:false carried explicitly", got)
	}
}
