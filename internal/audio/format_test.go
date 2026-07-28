package audio

import (
	"errors"
	"testing"

	"github.com/ebitengine/oto/v3"
)

func TestDefaultFormat(t *testing.T) {
	f := DefaultFormat()
	if f.SampleRate != 48000 {
		t.Errorf("SampleRate = %d, want 48000", f.SampleRate)
	}
	if f.Channels != 2 {
		t.Errorf("Channels = %d, want 2", f.Channels)
	}
}

func TestEngineErrNoDevice(t *testing.T) {
	orig := newOtoContext
	t.Cleanup(func() { newOtoContext = orig })
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, nil, errors.New("stub: no sound device")
	}

	_, err := NewEngine(DefaultFormat())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNoDevice) {
		t.Errorf("errors.Is(err, ErrNoDevice) = false; err = %v", err)
	}
}

func TestEngineCloseTwice(t *testing.T) {
	orig := newOtoContext
	t.Cleanup(func() { newOtoContext = orig })
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, nil, errors.New("stub: no device")
	}

	_, err := NewEngine(DefaultFormat())
	if err == nil {
		t.Skip("real device present; skip close-twice test in this environment")
	}

	e := &Engine{}
	if err2 := e.Close(); err2 != nil {
		t.Errorf("first Close: %v", err2)
	}
	if err2 := e.Close(); err2 != nil {
		t.Errorf("second Close: %v", err2)
	}
}

type mockPlayer struct{ paused bool }

func (p *mockPlayer) Pause() { p.paused = true }

func stubSeams(t *testing.T, readyClosed bool) {
	t.Helper()
	orig1 := newOtoContext
	orig2 := newOtoPlayer
	t.Cleanup(func() { newOtoContext = orig1; newOtoPlayer = orig2 })
	var ready chan struct{}
	if readyClosed {
		ready = make(chan struct{})
		close(ready)
	}
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, ready, nil
	}
	newOtoPlayer = func(_ *oto.Context, _ *Mixer) playerPauser {
		return &mockPlayer{}
	}
}

func TestNewEngine_SuccessNilReady(t *testing.T) {
	stubSeams(t, false)
	eng, err := NewEngine(DefaultFormat())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if eng.Mixer() == nil {
		t.Fatal("Mixer() returned nil")
	}
}

func TestNewEngine_SuccessClosedReady(t *testing.T) {
	stubSeams(t, true)
	eng, err := NewEngine(DefaultFormat())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if eng.Mixer() == nil {
		t.Fatal("Mixer() returned nil")
	}
	mp := eng.player.(*mockPlayer)
	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mp.paused {
		t.Fatal("player.Pause not called on Close")
	}
}
