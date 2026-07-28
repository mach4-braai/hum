package audio

import (
	"math"
	"sync"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

type envState uint8

const (
	envAttack envState = iota
	envSustain
	envRelease
	envDone
)

const (
	freqSmoothAlpha = 0.01
	gainSmoothAlpha = 0.005
	tremoloDepthMax = 0.10
	twoPi           = 2 * math.Pi
	invSqrt2        = 1.0 / math.Sqrt2
)

type Envelope struct {
	Attack  time.Duration
	Release time.Duration
}

type Osc struct {
	mu sync.Mutex
	sr float64

	phaseL float64
	phaseR float64

	curFreqL float64
	curFreqR float64
	tgtFreqL float64
	tgtFreqR float64
	baseFreq float64

	curGain  float64
	tgtGain  float64
	initGain float64

	harmonic     float64
	tremoloDepth float64
	tremoloHz    float64
	tremoloPhase float64
	detuneCents  float64
	width        float64

	state          envState
	attackSamples  float64
	releaseSamples float64
	envPos         float64
	peakGain       float64
}

func NewOsc(f Format, freq float64, gain float64, env Envelope) *Osc {
	sr := float64(f.SampleRate)
	o := &Osc{
		sr:             sr,
		baseFreq:       freq,
		curFreqL:       freq,
		curFreqR:       freq,
		tgtFreqL:       freq,
		tgtFreqR:       freq,
		tgtGain:        gain,
		initGain:       gain,
		attackSamples:  env.Attack.Seconds() * sr,
		releaseSamples: env.Release.Seconds() * sr,
		state:          envAttack,
	}
	if o.attackSamples <= 0 {
		o.state = envSustain
		o.curGain = gain
	}
	return o
}

func (o *Osc) SetFreq(freq float64) {
	o.mu.Lock()
	o.baseFreq = freq
	factor := o.width * o.detuneCents / 2400.0
	o.tgtFreqL = freq * math.Pow(2, factor)
	o.tgtFreqR = freq * math.Pow(2, -factor)
	o.mu.Unlock()
}

func (o *Osc) SetGain(gain float64) {
	if !(gain >= 0 && gain <= 1) {
		return
	}
	o.mu.Lock()
	o.tgtGain = gain
	o.mu.Unlock()
}

func (o *Osc) SetEnvelope(env Envelope) {
	o.mu.Lock()
	o.attackSamples = env.Attack.Seconds() * o.sr
	o.releaseSamples = env.Release.Seconds() * o.sr
	if o.state == envAttack && o.attackSamples <= 0 {
		o.state = envSustain
		o.envPos = 0
	}
	o.mu.Unlock()
}

func (o *Osc) SetExpression(e harmony.Expression, t theme.DroneSpec) {
	o.mu.Lock()
	o.harmonic = e.Intensity * t.Harmonic
	o.tremoloDepth = e.Tremolo
	o.tremoloHz = t.TremoloHz
	o.width = e.Width
	o.detuneCents = t.DetuneCents
	factor := e.Width * t.DetuneCents / 2400.0
	o.tgtFreqL = o.baseFreq * math.Pow(2, factor)
	o.tgtFreqR = o.baseFreq * math.Pow(2, -factor)
	o.mu.Unlock()
}

func (o *Osc) Release() {
	o.mu.Lock()
	if o.state == envRelease || o.state == envDone {
		o.mu.Unlock()
		return
	}
	o.peakGain = o.curGain
	o.envPos = 0
	o.state = envRelease
	o.mu.Unlock()
}

func (o *Osc) Mix(buf [][2]float32) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state == envDone {
		for i := range buf {
			buf[i] = [2]float32{}
		}
		return true
	}

	for i := range buf {
		switch o.state {
		case envAttack:
			o.curGain += o.initGain / o.attackSamples
			o.envPos++
			if o.envPos >= o.attackSamples {
				o.curGain = o.initGain
				o.state = envSustain
				o.envPos = 0
			}
		case envSustain:
			o.curGain += (o.tgtGain - o.curGain) * gainSmoothAlpha
		case envRelease:
			if o.releaseSamples > 0 {
				o.curGain -= o.peakGain / o.releaseSamples
			} else {
				o.curGain = 0
			}
			o.envPos++
			if o.envPos >= o.releaseSamples || o.curGain <= 0 {
				o.curGain = 0
				o.state = envDone
				for j := i; j < len(buf); j++ {
					buf[j] = [2]float32{}
				}
				return true
			}
		}

		o.curFreqL += (o.tgtFreqL - o.curFreqL) * freqSmoothAlpha
		o.curFreqR += (o.tgtFreqR - o.curFreqR) * freqSmoothAlpha

		tremoloScale := 1.0
		if o.tremoloHz > 0 && o.tremoloDepth > 0 {
			o.tremoloPhase += twoPi * o.tremoloHz / o.sr
			if o.tremoloPhase >= twoPi {
				o.tremoloPhase -= twoPi
			}
			tremoloScale = 1.0 + o.tremoloDepth*tremoloDepthMax*math.Sin(o.tremoloPhase)
		}

		phiL := twoPi * o.curFreqL / o.sr
		phiR := twoPi * o.curFreqR / o.sr

		sL := math.Sin(o.phaseL) + o.harmonic*math.Sin(2*o.phaseL)
		sR := math.Sin(o.phaseR) + o.harmonic*math.Sin(2*o.phaseR)

		amp := o.curGain * tremoloScale * invSqrt2

		buf[i][0] += float32(sL * amp)
		buf[i][1] += float32(sR * amp)

		o.phaseL += phiL
		if o.phaseL >= twoPi {
			o.phaseL -= twoPi
		}
		o.phaseR += phiR
		if o.phaseR >= twoPi {
			o.phaseR -= twoPi
		}
	}

	return false
}
