package audio

import (
	"math"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

func goertzel(samples []float64, targetHz, sampleRate float64) float64 {
	n := len(samples)
	k := targetHz * float64(n) / sampleRate
	omega := twoPi * k / float64(n)
	coeff := 2 * math.Cos(omega)
	s0, s1, s2 := 0.0, 0.0, 0.0
	for _, x := range samples {
		s0 = x + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}
	return math.Sqrt(s1*s1 + s2*s2 - coeff*s1*s2)
}

func readMixerLeft(m *Mixer, frames int) []float64 {
	p := make([]byte, frames*frameSize)
	m.Read(p)
	out := make([]float64, frames)
	for i := range out {
		bits := uint32(p[i*8+0]) | uint32(p[i*8+1])<<8 | uint32(p[i*8+2])<<16 | uint32(p[i*8+3])<<24
		out[i] = float64(math.Float32frombits(bits))
	}
	return out
}

func TestOscFrequency(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: 10 * time.Second})

	m := NewMixer(f)
	m.Add("v", osc)

	frames := f.SampleRate / 10
	samples := readMixerLeft(m, frames)

	p440 := goertzel(samples, 440, float64(f.SampleRate))
	p439 := goertzel(samples, 439, float64(f.SampleRate))
	p441 := goertzel(samples, 441, float64(f.SampleRate))

	if p440 <= p439 {
		t.Errorf("440 Hz power (%.3f) not greater than 439 Hz power (%.3f)", p440, p439)
	}
	if p440 <= p441 {
		t.Errorf("440 Hz power (%.3f) not greater than 441 Hz power (%.3f)", p440, p441)
	}
}

func TestOscPhaseContinuity(t *testing.T) {
	const threshold = 0.10

	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: 10 * time.Second})

	m := NewMixer(f)
	m.Add("v", osc)

	const bufFrames = 1024
	s1 := readMixerLeft(m, bufFrames)
	s2 := readMixerLeft(m, bufFrames)

	diff := math.Abs(s2[0] - s1[bufFrames-1])
	if diff > threshold {
		t.Errorf("phase discontinuity at buffer boundary: |s2[0] - s1[%d]| = %.6f > threshold %.3f",
			bufFrames-1, diff, threshold)
	}
}

func TestOscPhaseContinuityMidBufferFreqChange(t *testing.T) {
	const threshold = 0.10

	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: 10 * time.Second})

	buf := make([][2]float32, 512)
	osc.Mix(buf[:256])

	osc.SetFreq(880)
	osc.Mix(buf[256:])

	for i := 1; i < 512; i++ {
		diff := math.Abs(float64(buf[i][0]) - float64(buf[i-1][0]))
		if diff > threshold {
			t.Errorf("discontinuity at sample %d after freq change: %.6f > %.3f", i, diff, threshold)
			break
		}
	}
}

func TestOscReleaseYieldsDone(t *testing.T) {
	f := DefaultFormat()
	releaseTime := 50 * time.Millisecond
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: releaseTime})

	osc.Release()

	releaseSamples := int(releaseTime.Seconds() * float64(f.SampleRate))
	buf := make([][2]float32, releaseSamples+512)

	done := osc.Mix(buf)
	if !done {
		t.Error("Mix did not return done after reading past release duration")
	}
	for i := releaseSamples; i < len(buf); i++ {
		if buf[i][0] != 0 || buf[i][1] != 0 {
			t.Errorf("non-zero sample at %d after release complete: L=%v R=%v", i, buf[i][0], buf[i][1])
			break
		}
	}
}

func TestOscReleaseNotDoneBeforeDuration(t *testing.T) {
	f := DefaultFormat()
	releaseTime := 200 * time.Millisecond
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: releaseTime})

	osc.Release()

	halfSamples := int(releaseTime.Seconds() * float64(f.SampleRate) / 2)
	buf := make([][2]float32, halfSamples)
	done := osc.Mix(buf)
	if done {
		t.Error("Mix returned done before release duration elapsed")
	}
}

func TestOscReleaseIdempotent(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 100 * time.Millisecond})
	osc.Release()
	osc.Release()

	releaseSamples := int(0.1 * float64(f.SampleRate))
	buf := make([][2]float32, releaseSamples+128)
	done := osc.Mix(buf)
	if !done {
		t.Error("repeated Release() corrupted envelope; Mix never returned done")
	}
}

func TestOscSetGainNaNRejected(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})
	osc.SetGain(math.NaN())

	buf := make([][2]float32, 64)
	osc.Mix(buf)
	for i, fr := range buf {
		if math.IsNaN(float64(fr[0])) || math.IsNaN(float64(fr[1])) {
			t.Errorf("NaN in output at frame %d after SetGain(NaN)", i)
			break
		}
	}
}

func TestOscSetExpression(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})

	expr := harmony.Expression{Intensity: 0.5, Tremolo: 0.8, Width: 0.6}
	spec := theme.DroneSpec{
		Harmonic:    0.15,
		TremoloHz:   5.0,
		DetuneCents: 8.0,
	}
	osc.SetExpression(expr, spec)

	buf := make([][2]float32, 128)
	done := osc.Mix(buf)
	if done {
		t.Error("SetExpression caused early done")
	}
	for i, fr := range buf {
		if math.IsNaN(float64(fr[0])) || math.IsNaN(float64(fr[1])) {
			t.Errorf("NaN at frame %d after SetExpression", i)
			break
		}
	}
}

func TestOscDetuneSeparatesChannels(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: 0, Release: 10 * time.Second})

	expr := harmony.Expression{Width: 1.0}
	spec := theme.DroneSpec{DetuneCents: 20.0}
	osc.SetExpression(expr, spec)

	buf := make([][2]float32, 4800)
	osc.Mix(buf)

	var diffSum float64
	for _, fr := range buf {
		d := math.Abs(float64(fr[0]) - float64(fr[1]))
		diffSum += d
	}
	avgDiff := diffSum / float64(len(buf))
	if avgDiff < 0.001 {
		t.Errorf("L and R channels too similar with DetuneCents=20: avg diff = %.6f", avgDiff)
	}
}

func TestOscAttackRamps(t *testing.T) {
	f := DefaultFormat()
	attackDur := 10 * time.Millisecond
	osc := NewOsc(f, 440, 0.8, Envelope{Attack: attackDur, Release: 5 * time.Second})

	attackSamples := int(attackDur.Seconds() * float64(f.SampleRate))
	buf := make([][2]float32, attackSamples*2)
	osc.Mix(buf)

	firstAmp := math.Abs(float64(buf[0][0]))
	lastAmp := math.Abs(float64(buf[attackSamples-1][0]))
	if firstAmp >= lastAmp {
		t.Errorf("attack not ramping: first=%.6f, at-end=%.6f", firstAmp, lastAmp)
	}
}
