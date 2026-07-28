package renderer

import (
	"fmt"
	"sync"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

var (
	_ Renderer  = (*NopRenderer)(nil)
	_ Themeable = (*NopRenderer)(nil)
)

func init() {
	Register("nop", func(opts Options) (Renderer, error) {
		return NewNop(opts), nil
	})
}

type NopRenderer struct {
	mu       sync.Mutex
	opts     Options
	updates  []harmony.State
	triggers []harmony.Phrase
	volume   float64
	muted    bool
	closes   int
}

func NewNop(opts Options) *NopRenderer {
	opts = applyDefaults(opts)
	return &NopRenderer{
		opts:   opts,
		volume: opts.Volume,
		muted:  opts.Muted,
	}
}

func (n *NopRenderer) Name() string { return "nop" }

func (n *NopRenderer) SetTheme(t theme.Theme) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.opts.Theme = t
	return nil
}

func (n *NopRenderer) Theme() theme.Theme {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.opts.Theme
}

func (n *NopRenderer) Update(s harmony.State) error {
	cp := harmony.State{Voices: make([]harmony.VoiceState, len(s.Voices))}
	copy(cp.Voices, s.Voices)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.updates = append(n.updates, cp)
	return nil
}

func (n *NopRenderer) Trigger(p harmony.Phrase) error {
	cp := harmony.Phrase{Kind: p.Kind, Notes: make([]harmony.Note, len(p.Notes))}
	copy(cp.Notes, p.Notes)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.triggers = append(n.triggers, cp)
	return nil
}

func (n *NopRenderer) SetVolume(v float64) error {
	if !(v >= 0 && v <= 1) {
		return fmt.Errorf("renderer: volume %v out of range [0, 1]", v)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.volume = v
	return nil
}

func (n *NopRenderer) SetMuted(m bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.muted = m
	return nil
}

func (n *NopRenderer) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closes++
	return nil
}

func (n *NopRenderer) Updates() []harmony.State {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]harmony.State, len(n.updates))
	for i, s := range n.updates {
		voices := make([]harmony.VoiceState, len(s.Voices))
		copy(voices, s.Voices)
		out[i] = harmony.State{Voices: voices}
	}
	return out
}

func (n *NopRenderer) Triggers() []harmony.Phrase {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]harmony.Phrase, len(n.triggers))
	for i, p := range n.triggers {
		notes := make([]harmony.Note, len(p.Notes))
		copy(notes, p.Notes)
		out[i] = harmony.Phrase{Kind: p.Kind, Notes: notes}
	}
	return out
}

func (n *NopRenderer) Volume() float64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.volume
}

func (n *NopRenderer) Muted() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.muted
}

func (n *NopRenderer) Closes() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closes
}
