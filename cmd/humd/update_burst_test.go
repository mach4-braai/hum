package main

import (
	"syscall"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestApplyEventUpdateBurstAllocatesNoExtraVoice(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "burst"}
	if resp := send(t, socket, protocol.Request{Event: &started})[0]; !resp.OK {
		t.Fatalf("session.started = %+v", resp)
	}

	before := statusOf(t, socket)
	if len(before.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(before.Sessions))
	}
	pitch := before.Sessions[0].Pitch

	for i := range 10 {
		updated := protocol.Event{Event: protocol.SessionUpdated, ID: "burst"}
		if resp := send(t, socket, protocol.Request{Event: &updated})[0]; !resp.OK {
			t.Fatalf("update %d = %+v", i, resp)
		}
	}

	after := statusOf(t, socket)
	if len(after.Sessions) != 1 {
		t.Fatalf("session count = %d after ten updates, want 1", len(after.Sessions))
	}
	if after.Sessions[0].State != "active" {
		t.Errorf("state = %q after ten updates, want active: update must never end a session", after.Sessions[0].State)
	}
	if after.Sessions[0].Pitch != pitch {
		t.Errorf("pitch = %q, want %q unchanged: updates route to expression, not to new notes", after.Sessions[0].Pitch, pitch)
	}
	if after.SoundingVoices != 1 {
		t.Errorf("sounding voices = %d after ten updates, want 1", after.SoundingVoices)
	}
	if after.Sessions[0].Updates != 10 {
		t.Errorf("updates = %d, want 10", after.Sessions[0].Updates)
	}
}
