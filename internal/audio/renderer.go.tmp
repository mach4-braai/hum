package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
)

var newAudioRenderer = func(opts renderer.Options) (renderer.Renderer, error) {
	f := Format{SampleRate: opts.SampleRate, Channels: 2}
	if f.SampleRate == 0 {
		f = DefaultFormat()
	}
	eng, err := NewEngine(f)
	if err != nil {
		return nil, err
	}
	r := newRendererWithMixer(eng.Mixer(), f, opts)
	r.engine = eng
	return r, nil
}

func init() {
	renderer.Register("audio", newAudioRenderer)
}

const (
	muteRampDuration = 50 * time.Millisecond
	maxPhraseVoices  = 16
	fallbackAttack   = 100 * time.Millisecond
	fallbackRelease  = 200 * time.Millisecond
	muteRampSteps    = 20
)

type activeVoice struct {
	mixerID string
	osc     *Osc
	last    harmony.VoiceState
}

type AudioRenderer struct {
	mu           sync.Mutex
	mixer        *Mixer
	engine       *Engine
	format       Format
	active       map[string]*activeVoice
	volume       float64
	muted        bool
	th           theme.Theme
	closed       bool
	seq          uint64
	phraseIDs    []string
	phraseSeq    uint64
	rampGen      uint64
	rampDuration time.Duration
}

var (
	_ renderer.Renderer  = (*AudioRenderer)(nil)
	_ renderer.Themeable = (*AudioRenderer)(nil)
	_ renderer.Sampled   = (*AudioRenderer)(nil)
)

func newRendererWithMixer(m *Mixer, f Format, opts renderer.Options) *AudioRenderer {
	r := &AudioRenderer{
		mixer:        m,
		format:       f,
		active:       make(map[string]*activeVoice),
		volume:       opts.Volume,
		muted:        opts.Muted,
		th:           opts.Theme,
		rampDuration: muteRampDuration,
	}
	if !opts.Muted {
		m.SetGain(opts.Volume)
	} else {
		m.SetGain(0)
	}
	return r
}

func (r *AudioRenderer) Name() string { return "audio" }

func (r *AudioRenderer) SampleRate() int { return r.format.SampleRate }

func (r *AudioRenderer) SetTheme(t theme.Theme) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.th = t
	env := droneEnvelope(t.Drone)
	for _, av := range r.active {
		av.osc.SetEnvelope(env)
		av.osc.SetGain(t.Drone.Gain)
		av.osc.SetExpression(av.last.Expression, t.Drone)
	}
	return nil
}

func (r *AudioRenderer) Update(s harmony.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	incoming := make(map[string]harmony.VoiceState, len(s.Voices))
	for _, vs := range s.Voices {
		incoming[vs.Voice.SessionID] = vs
	}

	for sid, av := range r.active {
		if _, ok := incoming[sid]; !ok {
			av.osc.Release()
			delete(r.active, sid)
		}
	}

	for sid, vs := range incoming {
		if av, ok := r.active[sid]; ok {
			if av.last != vs {
				av.osc.SetFreq(vs.Voice.Pitch.Freq())
				av.osc.SetGain(r.th.Drone.Gain)
				av.osc.SetExpression(vs.Expression, r.th.Drone)
				av.last = vs
			}
		} else {
			env := droneEnvelope(r.th.Drone)
			osc := NewOsc(r.format, vs.Voice.Pitch.Freq(), r.th.Drone.Gain, env)
			osc.SetExpression(vs.Expression, r.th.Drone)
			r.seq++
			mid := fmt.Sprintf("drone/%d", r.seq)
			r.mixer.Add(mid, osc)
			r.active[sid] = &activeVoice{mixerID: mid, osc: osc, last: vs}
		}
	}

	return nil
}

func droneEnvelope(d theme.DroneSpec) Envelope {
	attack := time.Duration(d.Attack * float64(time.Second))
	release := time.Duration(d.Release * float64(time.Second))
	if attack <= 0 {
		attack = fallbackAttack
	}
	if release <= 0 {
		release = fallbackRelease
	}
	return Envelope{Attack: attack, Release: release}
}

func (r *AudioRenderer) Trigger(p harmony.Phrase) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	for _, note := range p.Notes {
		src := newPhraseSource(r.format, note, r.th.Phrases)
		r.phraseSeq++
		id := fmt.Sprintf("phrase/%d", r.phraseSeq)
		r.schedulePhraseSource(id, src)
	}

	return nil
}

func (r *AudioRenderer) schedulePhraseSource(id string, src *phraseSource) {
	var alive []string
	for _, pid := range r.phraseIDs {
		if r.mixer.Has(pid) {
			alive = append(alive, pid)
		}
	}
	r.phraseIDs = alive

	for len(r.phraseIDs) >= maxPhraseVoices {
		oldest := r.phraseIDs[0]
		r.phraseIDs = r.phraseIDs[1:]
		r.mixer.Remove(oldest)
	}

	r.mixer.Add(id, src)
	r.phraseIDs = append(r.phraseIDs, id)
}

func (r *AudioRenderer) SetVolume(v float64) error {
	if !(v >= 0 && v <= 1) {
		return fmt.Errorf("audio: volume %v out of range [0, 1]", v)
	}
	r.mu.Lock()
	r.volume = v
	muted := r.muted
	r.mu.Unlock()
	if !muted {
		r.mixer.SetGain(v)
	}
	return nil
}

func (r *AudioRenderer) SetMuted(m bool) error {
	r.mu.Lock()
	if r.muted == m {
		r.mu.Unlock()
		return nil
	}
	r.muted = m
	r.rampGen++
	gen := r.rampGen
	rampDur := r.rampDuration
	if m {
		from := r.mixer.Gain()
		r.mu.Unlock()
		go r.doRamp(from, 0, rampDur, gen)
	} else {
		target := r.volume
		r.mu.Unlock()
		r.mixer.SetGain(target)
	}
	return nil
}

func (r *AudioRenderer) doRamp(from, to float64, dur time.Duration, gen uint64) {
	stepDur := dur / muteRampSteps
	for i := 1; i <= muteRampSteps; i++ {
		time.Sleep(stepDur)
		r.mu.Lock()
		if r.rampGen != gen {
			r.mu.Unlock()
			return
		}
		frac := float64(i) / muteRampSteps
		r.mixer.SetGain(from + (to-from)*frac)
		r.mu.Unlock()
	}
}

func (r *AudioRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	r.rampGen++

	if r.engine != nil {
		return r.engine.Close()
	}
	return nil
}
