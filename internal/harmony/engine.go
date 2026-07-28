package harmony

import (
	"errors"
	"time"

	"github.com/mach4-braai/hum/internal/session"
)

var ErrRetuneBusy = errors.New("retune: voices are sounding")

type VoiceState struct {
	Voice
	Expression Expression
}

type State struct {
	Voices []VoiceState
}

type PhraseKind string

const (
	PhraseCompletion PhraseKind = "completion"
	PhraseFailure    PhraseKind = "failure"
	PhraseCancelled  PhraseKind = "cancelled"
)

type Note struct {
	Pitch    Pitch
	Offset   time.Duration
	Duration time.Duration
	Gain     float64
}

type Phrase struct {
	Kind  PhraseKind
	Notes []Note
}

type PhraseSpec struct {
	CompletionOctaves  int
	CompletionDuration time.Duration
	CompletionGain     float64
	FailureInterval    int
	FailureDuration    time.Duration
	FailureGain        float64
	CancelledSounds    bool
	CancelledDuration  time.Duration
	CancelledGain      float64
}

func DefaultPhraseSpec() PhraseSpec {
	return PhraseSpec{
		CompletionOctaves:  2,
		CompletionDuration: 500 * time.Millisecond,
		CompletionGain:     0.8,
		FailureInterval:    -3,
		FailureDuration:    800 * time.Millisecond,
		FailureGain:        0.5,
		CancelledSounds:    false,
		CancelledDuration:  400 * time.Millisecond,
		CancelledGain:      0.3,
	}
}

type Engine struct {
	root  Pitch
	scale Scale
	spec  PhraseSpec
	alloc *Allocator
	exprs map[string]*exprTracker
}

func NewEngine(root Pitch, scale Scale, spec PhraseSpec) *Engine {
	return &Engine{
		root:  root,
		scale: scale,
		spec:  spec,
		alloc: NewAllocator(root, scale),
		exprs: make(map[string]*exprTracker),
	}
}

func (e *Engine) Apply(c session.Change) (State, []Phrase) {
	switch c.Kind {
	case session.ChangeAdded:
		e.alloc.Acquire(c.Session.ID)
		e.exprs[c.Session.ID] = &exprTracker{
			agents: agentsFromMetadata(c.Session.Metadata),
		}
		return e.buildState(), nil

	case session.ChangeUpdated:
		if t, ok := e.exprs[c.Session.ID]; ok {
			t.record()
			t.agents = agentsFromMetadata(c.Session.Metadata)
		}
		return e.buildState(), nil

	case session.ChangeEnded:
		voice, hasVoice := e.alloc.VoiceFor(c.Session.ID)
		var phrases []Phrase
		if hasVoice {
			phrases = e.buildPhrases(c.Session, voice)
		}
		e.alloc.Release(c.Session.ID)
		delete(e.exprs, c.Session.ID)
		return e.buildState(), phrases
	}
	return e.buildState(), nil
}

func (e *Engine) buildState() State {
	voices := e.alloc.Voices()
	vs := make([]VoiceState, len(voices))
	for i, v := range voices {
		vs[i] = VoiceState{Voice: v, Expression: e.exprs[v.SessionID].current()}
	}
	return State{Voices: vs}
}

func (e *Engine) buildPhrases(sess session.Session, voice Voice) []Phrase {
	switch sess.State {
	case session.StateCompleted:
		return []Phrase{{
			Kind: PhraseCompletion,
			Notes: []Note{{
				Pitch:    voice.Pitch.Transpose(e.spec.CompletionOctaves * 12),
				Offset:   0,
				Duration: e.spec.CompletionDuration,
				Gain:     e.spec.CompletionGain,
			}},
		}}
	case session.StateFailed:
		return []Phrase{{
			Kind: PhraseFailure,
			Notes: []Note{
				{
					Pitch:    voice.Pitch,
					Offset:   0,
					Duration: e.spec.FailureDuration,
					Gain:     e.spec.FailureGain,
				},
				{
					Pitch:    voice.Pitch.Transpose(e.spec.FailureInterval),
					Offset:   e.spec.FailureDuration,
					Duration: e.spec.FailureDuration,
					Gain:     e.spec.FailureGain,
				},
			},
		}}
	case session.StateCancelled:
		if e.spec.CancelledSounds {
			return []Phrase{{
				Kind: PhraseCancelled,
				Notes: []Note{{
					Pitch:    voice.Pitch,
					Offset:   0,
					Duration: e.spec.CancelledDuration,
					Gain:     e.spec.CancelledGain,
				}},
			}}
		}
	}
	return nil
}

func (e *Engine) Retune(root Pitch, scale Scale) error {
	if e.alloc.Active() > 0 {
		return ErrRetuneBusy
	}
	e.root = root
	e.scale = scale
	e.alloc = NewAllocator(root, scale)
	return nil
}

func (e *Engine) Tuning() (Pitch, Scale) {
	return e.root, e.scale
}
