package audio

import (
	"math"

	"github.com/mach4-braai/hum/internal/theme"
)

const (
	ensembleMax     = 4
	toneControlRate = 64
	driftDepth      = 0.5
	driftSpread     = 0.37
	filterCeiling   = 0.45
)

type Tone struct {
	Partials   int
	CutoffHz   float64
	Brightness float64
	Voices     int
	Cents      float64
	DriftHz    float64
}

func ToneOf(t theme.Theme) Tone {
	if t.Waveform != theme.WaveformStrings {
		return Tone{}
	}
	return Tone{
		Partials:   t.Drone.Partials,
		CutoffHz:   t.Drone.CutoffHz,
		Brightness: t.Drone.Brightness,
		Voices:     t.Drone.EnsembleVoices,
		Cents:      t.Drone.EnsembleCents,
		DriftHz:    t.Drone.EnsembleDriftHz,
	}
}

type ensembleVoice struct {
	phaseL     float64
	phaseR     float64
	curL       float64
	curR       float64
	tgtL       float64
	tgtR       float64
	bandL      int
	bandR      int
	offset     float64
	driftPhase float64
	driftStep  float64
}

func ensembleOffset(index, voices int, cents float64) float64 {
	if voices < 2 {
		return 0
	}
	return (float64(index)/float64(voices-1) - 0.5) * cents
}

func (o *Osc) SetTone(t Tone) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.tone = t
	if t.Voices < 1 {
		o.table = nil
		o.voices = 0
		return
	}

	o.voices = min(t.Voices, ensembleMax)
	o.table = loadWavetable(int(o.sr), t.Partials)
	for i := range o.voices {
		e := &o.ens[i]
		e.offset = ensembleOffset(i, o.voices, t.Cents)
		e.driftStep = twoPi * t.DriftHz * (1 + driftSpread*float64(i)) * toneControlRate / o.sr
		e.driftPhase = twoPi * float64(i) / float64(o.voices)
		e.phaseL = 0
		e.phaseR = 0
		e.curL = o.baseFreq
		e.curR = o.baseFreq
	}
	o.spread = 1 / math.Sqrt(float64(o.voices))
	o.control = 0
	o.updateEnsemble()
	o.updateFilter()
}

func (o *Osc) updateEnsemble() {
	widthCents := o.width * o.detuneCents / 2
	for i := range o.voices {
		e := &o.ens[i]
		e.driftPhase = math.Mod(e.driftPhase+e.driftStep, twoPi)
		cents := e.offset + driftDepth*o.tone.Cents*math.Sin(e.driftPhase)
		e.tgtL = o.baseFreq * math.Exp2((cents+widthCents)/1200)
		e.tgtR = o.baseFreq * math.Exp2((cents-widthCents)/1200)
		e.bandL = tableBand(math.Max(e.curL, e.tgtL))
		e.bandR = tableBand(math.Max(e.curR, e.tgtR))
	}
}

func (o *Osc) updateFilter() {
	cutoff := o.tone.CutoffHz * math.Exp2(o.intensity*o.tone.Brightness)
	if ceiling := filterCeiling * o.sr; cutoff > ceiling {
		cutoff = ceiling
	}
	o.lpA = 1 - math.Exp(-twoPi*cutoff/o.sr)
}

func (o *Osc) lowPass(channel int, x float64) float64 {
	poles := &o.lp[channel]
	poles[0] += o.lpA * (x - poles[0])
	poles[1] += o.lpA * (poles[0] - poles[1])
	return poles[1]
}

func advancePhase(phase, step float64) float64 {
	phase += step
	if phase >= twoPi {
		phase -= twoPi
	}
	return phase
}

func (o *Osc) ensembleSample() (float64, float64) {
	o.control--
	if o.control <= 0 {
		o.control = toneControlRate
		o.updateEnsemble()
	}

	sumL, sumR := 0.0, 0.0
	for i := range o.voices {
		e := &o.ens[i]
		e.curL += (e.tgtL - e.curL) * freqSmoothAlpha
		e.curR += (e.tgtR - e.curR) * freqSmoothAlpha
		sumL += o.table.at(e.bandL, e.phaseL)
		sumR += o.table.at(e.bandR, e.phaseR)
		e.phaseL = advancePhase(e.phaseL, twoPi*e.curL/o.sr)
		e.phaseR = advancePhase(e.phaseR, twoPi*e.curR/o.sr)
	}

	return o.lowPass(0, sumL*o.spread), o.lowPass(1, sumR*o.spread)
}
