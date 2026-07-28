package audio

import (
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

const (
	fallbackPhraseAttack = 5 * time.Millisecond
	fallbackPhraseDecay  = 50 * time.Millisecond
)

type phraseSource struct {
	osc             *Osc
	offsetSamples   int
	durationSamples int
	released        bool
}

var _ Source = (*phraseSource)(nil)

func newPhraseSource(f Format, note harmony.Note, phrases theme.PhrasesSpec) *phraseSource {
	attack := time.Duration(phrases.Attack * float64(time.Second))
	decay := time.Duration(phrases.Decay * float64(time.Second))
	if attack <= 0 {
		attack = fallbackPhraseAttack
	}
	if decay <= 0 {
		decay = fallbackPhraseDecay
	}
	env := Envelope{Attack: attack, Release: decay}
	osc := NewOsc(f, note.Pitch.Freq(), note.Gain, env)
	offsetSamples := int(note.Offset.Seconds() * float64(f.SampleRate))
	durSamples := int(note.Duration.Seconds() * float64(f.SampleRate))
	return &phraseSource{
		osc:             osc,
		offsetSamples:   offsetSamples,
		durationSamples: durSamples,
	}
}

func (p *phraseSource) Mix(buf [][2]float32) bool {
	if p.offsetSamples > 0 {
		skip := p.offsetSamples
		if skip > len(buf) {
			skip = len(buf)
		}
		p.offsetSamples -= skip
		buf = buf[skip:]
		if len(buf) == 0 {
			return false
		}
	}
	if !p.released {
		if p.durationSamples <= 0 {
			p.released = true
			p.osc.Release()
		} else {
			p.durationSamples -= len(buf)
			if p.durationSamples <= 0 {
				p.released = true
				p.osc.Release()
			}
		}
	}
	return p.osc.Mix(buf)
}
