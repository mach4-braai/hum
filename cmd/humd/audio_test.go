package main

import (
	"encoding/json"
	"strings"
	"syscall"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
)

func audioTestPayload(t *testing.T, socket string) protocol.AudioTestPayload {
	t.Helper()

	response := send(t, socket, protocol.Request{Command: protocol.CmdAudioTest})[0]
	if !response.OK {
		t.Fatalf("audio.test = %+v, want ok", response)
	}
	var got protocol.AudioTestPayload
	if err := json.Unmarshal(response.Data, &got); err != nil {
		t.Fatalf("decode audio.test payload: %v", err)
	}
	return got
}

func TestAudioTestPlaysAToneThroughTheRenderer(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	got := audioTestPayload(t, socket)

	if !got.Played {
		t.Errorf("played = false for the %q renderer, want a real renderer reported as audible", got.Renderer)
	}
	if got.Renderer != "recorder" {
		t.Errorf("renderer = %q, want recorder", got.Renderer)
	}
	if got.Seconds != audioTestDuration.Seconds() {
		t.Errorf("seconds = %v, want %v", got.Seconds, audioTestDuration.Seconds())
	}
	if history := strings.Join(rec.history(), ","); !strings.Contains(history, "trigger/test") {
		t.Errorf("call history %q is missing the test phrase", history)
	}
}

func TestAudioTestReportsTheNopRendererAsSilent(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = renderer.NewNop(renderer.Options{})
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	got := audioTestPayload(t, socket)

	if got.Played {
		t.Error("played = true under the nop renderer, want the client told nothing was audible")
	}
	if got.Renderer != "nop" {
		t.Errorf("renderer = %q, want nop", got.Renderer)
	}
}

func TestAudioTestReportsAMutedDaemonAsSilent(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if response := send(t, socket, protocol.Request{Command: protocol.CmdMute})[0]; !response.OK {
		t.Fatalf("mute = %+v, want ok", response)
	}

	got := audioTestPayload(t, socket)

	if got.Played {
		t.Error("played = true while muted, want the client told nothing was audible")
	}
	if !got.Muted {
		t.Error("muted = false, want the mute reported as the reason for silence")
	}
}

func TestAudioTestSurfacesARendererFailure(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &triggerErrRenderer{}
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	response := send(t, socket, protocol.Request{Command: protocol.CmdAudioTest})[0]

	if response.OK {
		t.Fatalf("audio.test = %+v, want the renderer failure reported", response)
	}
	if !strings.Contains(response.Error, "trigger failure") {
		t.Errorf("error = %q, want the renderer's own message", response.Error)
	}
}

func TestStatusReportsTheSoundingPitchOfEachSession(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	send(t, socket, event(protocol.SessionStarted, "s1"))
	status := statusOf(t, socket)

	if len(status.Sessions) != 1 {
		t.Fatalf("status reported %d sessions, want 1", len(status.Sessions))
	}
	if status.Sessions[0].Pitch != "D2" {
		t.Errorf("pitch = %q, want D2 so an operator can correlate what they hear", status.Sessions[0].Pitch)
	}

	send(t, socket, event(protocol.SessionCompleted, "s1"))
	status = statusOf(t, socket)

	if len(status.Sessions) != 1 {
		t.Fatalf("status reported %d sessions after completion, want 1 still listed", len(status.Sessions))
	}
	if status.Sessions[0].Pitch != "" {
		t.Errorf("pitch = %q for a terminal session, want it empty once the voice is released", status.Sessions[0].Pitch)
	}
}

func TestStatusReportsTheDaemonVersionAndSampleRate(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = renderer.NewNop(renderer.Options{SampleRate: 44100})
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	status := statusOf(t, socket)

	if status.Version != version {
		t.Errorf("version = %q, want the daemon's own %q so hum doctor can compare builds", status.Version, version)
	}
	if status.SampleRate != 44100 {
		t.Errorf("sample rate = %d, want 44100 from the renderer", status.SampleRate)
	}
}

func TestStatusReportsZeroSampleRateForARendererThatCannotSayOne(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	if status := statusOf(t, socket); status.SampleRate != 0 {
		t.Errorf("sample rate = %d for a renderer that reports none, want 0 rather than an invented default", status.SampleRate)
	}
}
