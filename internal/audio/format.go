package audio

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/oto/v3"
)

var ErrNoDevice = errors.New("no audio device available")

type Format struct {
	SampleRate int
	Channels   int
}

func DefaultFormat() Format {
	return Format{SampleRate: 48000, Channels: 2}
}

type playerPauser interface{ Pause() }

var newOtoContext = oto.NewContext

var newOtoPlayer = func(ctx *oto.Context, m *Mixer) playerPauser {
	p := ctx.NewPlayer(m)
	p.Play()
	return p
}

type Engine struct {
	mu     sync.Mutex
	ctx    *oto.Context
	player playerPauser
	mixer  *Mixer
	closed bool
}

func NewEngine(f Format) (*Engine, error) {
	m := NewMixer(f)
	opts := &oto.NewContextOptions{
		SampleRate:   f.SampleRate,
		ChannelCount: f.Channels,
		Format:       oto.FormatFloat32LE,
	}
	ctx, ready, err := newOtoContext(opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoDevice, err)
	}
	if ready != nil {
		<-ready
	}
	player := newOtoPlayer(ctx, m)
	return &Engine{ctx: ctx, player: player, mixer: m}, nil
}

func (e *Engine) Mixer() *Mixer {
	return e.mixer
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.player != nil {
		e.player.Pause()
	}
	return nil
}
