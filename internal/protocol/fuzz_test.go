package protocol

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func FuzzDecoder(f *testing.F) {
	const pre = `{"event":"session.started","id":"1","title":"`
	const suf = `"}`
	exact := pre + strings.Repeat("x", MaxMessageLen-len(pre)-len(suf)) + suf
	over := pre + strings.Repeat("x", MaxMessageLen-len(pre)-len(suf)+1) + suf

	f.Add([]byte(`{"event":"session.started","id":"1","title":"build"}` + "\n"))
	f.Add([]byte("\n"))
	f.Add([]byte("not a json object\n"))
	f.Add([]byte(exact + "\n"))
	f.Add([]byte(over + "\n"))
	f.Add([]byte(`{"event":"session.started","id":"1"}` + "\n" + `{"event":"session.completed","id":"2"}` + "\n"))
	f.Add([]byte(`{"event":"session.started","id":` + "\n"))
	f.Add([]byte(`{"event":"` + string([]byte{0x80, 0xbf}) + `","id":"1"}` + "\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		event, err := NewDecoder(bytes.NewReader(data)).Decode()
		if err != nil {
			return
		}

		var enc bytes.Buffer
		if encErr := NewEncoder(&enc).Encode(event); encErr != nil {
			if !errors.Is(encErr, ErrMessageTooLarge) {
				t.Fatalf("Encode of a decoded event failed with %v, want nil or ErrMessageTooLarge; event %+v from %q", encErr, event, data)
			}
			return
		}

		event2, err2 := NewDecoder(&enc).Decode()
		if err2 != nil {
			t.Fatalf("an event this package encoded does not decode: %v; event %+v", err2, event)
		}
		if !reflect.DeepEqual(event, event2) {
			t.Fatalf("round-trip mismatch: decoded %+v, re-decoded %+v; input %q", event, event2, data)
		}
	})
}
