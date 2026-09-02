package audio

import (
	"math"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

const stringsThreshold = 0.35

func loadTheme(t *testing.T, name string) theme.Theme {
	t.Helper()
	t.Setenv("HUM_HOME", t.TempDir())
	th, err := theme.Load(name)
	if err != nil {
		t.Fatalf("load theme %q: %v", name, err)
	}
	return th
}

func stringsOsc(t *testing.T, f Format, freq, gain float64, expr harmony.Expression) *Osc {
	t.Helper()
	th := loadTheme(t, "orchestra")
	osc := NewOsc(f, freq, gain, Envelope{Attack: 0, Release: time.Minute})
	osc.SetTone(ToneOf(th))
	osc.SetExpression(expr, th.Drone)
	return osc
}

func channels(osc *Osc, frames int) ([]float64, []float64) {
	buf := make([][2]float32, frames)
	osc.Mix(buf)
	left := make([]float64, frames)
	right := make([]float64, frames)
	for i, fr := range buf {
		left[i] = float64(fr[0])
		right[i] = float64(fr[1])
	}
	return left, right
}

func rms(samples []float64) float64 {
	sum := 0.0
	for _, s := range samples {
		sum += s * s
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func TestStringsHoldsTheLevelOfTheSineItReplaces(t *testing.T) {
	const tolerance = 3.0

	f := DefaultFormat()
	sine := loadTheme(t, "minimal")
	strings := loadTheme(t, "orchestra")
	expr := harmony.Expression{Intensity: 1, Tremolo: 0, Width: 1}

	for _, freq := range []float64{130.81, 261.63, 440} {
		want := NewOsc(f, freq, 0.5, Envelope{Attack: 0, Release: time.Minute})
		want.SetTone(ToneOf(sine))
		want.SetExpression(expr, sine.Drone)

		got := NewOsc(f, freq, 0.5, Envelope{Attack: 0, Release: time.Minute})
		got.SetTone(ToneOf(strings))
		got.SetExpression(expr, strings.Drone)

		frames := 8 * f.SampleRate
		sineLeft, _ := channels(want, frames)
		stringsLeft, _ := channels(got, frames)

		delta := 20 * math.Log10(rms(stringsLeft)/rms(sineLeft))
		if math.Abs(delta) > tolerance {
			t.Errorf("%.2f Hz: strings level is %+.2f dB against the sine, want within %.1f dB",
				freq, delta, tolerance)
		}
	}
}

func TestStringsPhaseContinuityAcrossBuffers(t *testing.T) {
	f := DefaultFormat()
	osc := stringsOsc(t, f, 440, 0.8, harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1})

	const bufFrames = 1024
	first, _ := channels(osc, bufFrames)
	second, _ := channels(osc, bufFrames)

	diff := math.Abs(second[0] - first[bufFrames-1])
	if diff > stringsThreshold {
		t.Errorf("discontinuity at buffer boundary: %.6f > %.3f", diff, stringsThreshold)
	}
}

func TestStringsPhaseContinuityAcrossAFrequencyChange(t *testing.T) {
	f := DefaultFormat()
	osc := stringsOsc(t, f, 440, 0.8, harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1})

	buf := make([][2]float32, 4096)
	osc.Mix(buf[:2048])
	osc.SetFreq(880)
	osc.Mix(buf[2048:])

	for i := 1; i < len(buf); i++ {
		diff := math.Abs(float64(buf[i][0]) - float64(buf[i-1][0]))
		if diff > stringsThreshold {
			t.Fatalf("discontinuity at sample %d after a frequency change: %.6f > %.3f",
				i, diff, stringsThreshold)
		}
	}
}

func TestStringsCarriesOddAndEvenPartials(t *testing.T) {
	f := DefaultFormat()
	sr := float64(f.SampleRate)
	freq := 220.0

	osc := stringsOsc(t, f, freq, 0.5, harmony.Expression{Intensity: 1})
	left, _ := channels(osc, f.SampleRate/4)

	fundamental := goertzel(left, freq, sr)
	for _, n := range []float64{2, 3, 4, 5} {
		partial := goertzel(left, n*freq, sr)
		if partial <= 0.01*fundamental {
			t.Errorf("partial %.0f is %.4f against a fundamental of %.4f, want an audible stack",
				n, partial, fundamental)
		}
	}
}

func TestSinePathCarriesNoThirdPartial(t *testing.T) {
	f := DefaultFormat()
	sr := float64(f.SampleRate)
	freq := 220.0
	th := loadTheme(t, "minimal")

	osc := NewOsc(f, freq, 0.5, Envelope{Attack: 0, Release: time.Minute})
	osc.SetTone(ToneOf(th))
	osc.SetExpression(harmony.Expression{Intensity: 1}, th.Drone)
	left, _ := channels(osc, f.SampleRate/4)

	fundamental := goertzel(left, freq, sr)
	third := goertzel(left, 3*freq, sr)
	if third > 0.01*fundamental {
		t.Errorf("third partial is %.4f against a fundamental of %.4f, want none from a sine theme",
			third, fundamental)
	}
}

func TestEnsembleMovementModulatesTheLevel(t *testing.T) {
	f := DefaultFormat()
	th := loadTheme(t, "orchestra")
	tone := ToneOf(th)

	spread := func(voices int) float64 {
		single := tone
		single.Voices = voices
		osc := NewOsc(f, 220, 0.5, Envelope{Attack: 0, Release: time.Minute})
		osc.SetTone(single)
		osc.SetExpression(harmony.Expression{Intensity: 1}, th.Drone)

		low, high := math.Inf(1), 0.0
		for range 16 {
			left, _ := channels(osc, f.SampleRate/2)
			level := rms(left)
			low = math.Min(low, level)
			high = math.Max(high, level)
		}
		return high / low
	}

	unison := spread(1)
	ensemble := spread(tone.Voices)

	if unison > 1.02 {
		t.Errorf("a single copy swings by %.3f, want a steady level", unison)
	}
	if ensemble < 1.1 {
		t.Errorf("%d detuned copies swing by %.3f, want audible ensemble movement",
			tone.Voices, ensemble)
	}
}

func TestBrightnessOpensWithIntensity(t *testing.T) {
	f := DefaultFormat()
	sr := float64(f.SampleRate)
	freq := 220.0

	tilt := func(intensity float64) float64 {
		osc := stringsOsc(t, f, freq, 0.5, harmony.Expression{Intensity: intensity})
		left, _ := channels(osc, f.SampleRate/4)
		return goertzel(left, 10*freq, sr) / goertzel(left, freq, sr)
	}

	dark := tilt(0)
	bright := tilt(1)
	if bright <= dark {
		t.Errorf("high partials at intensity 1 are %.5f against %.5f at intensity 0, want brighter",
			bright, dark)
	}
}

func TestStringsWidthSeparatesTheChannels(t *testing.T) {
	f := DefaultFormat()
	osc := stringsOsc(t, f, 220, 0.5, harmony.Expression{Intensity: 1, Width: 1})

	left, right := channels(osc, 4*f.SampleRate)

	identical := true
	for i := range left {
		if left[i] != right[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("left and right are sample-identical at Width 1, want a detuned pair")
	}
}

func TestToneOfReadsOnlyAStringsTheme(t *testing.T) {
	sine := loadTheme(t, "minimal")
	if got := ToneOf(sine); got != (Tone{}) {
		t.Errorf("ToneOf(minimal) = %+v, want the zero tone", got)
	}

	strings := loadTheme(t, "orchestra")
	want := Tone{
		Partials:   strings.Drone.Partials,
		CutoffHz:   strings.Drone.CutoffHz,
		Brightness: strings.Drone.Brightness,
		Voices:     strings.Drone.EnsembleVoices,
		Cents:      strings.Drone.EnsembleCents,
		DriftHz:    strings.Drone.EnsembleDriftHz,
	}
	if got := ToneOf(strings); got != want {
		t.Errorf("ToneOf(orchestra) = %+v, want %+v", got, want)
	}
}

func TestSetToneWithoutVoicesKeepsTheSinePath(t *testing.T) {
	f := DefaultFormat()

	plain := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: time.Minute})
	toned := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: time.Minute})
	toned.SetTone(Tone{Partials: 8, CutoffHz: 1500})

	want, _ := channels(plain, 512)
	got, _ := channels(toned, 512)

	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("sample %d = %v, want the sine value %v: a tone with no voices must not switch waveform",
				i, got[i], want[i])
		}
	}
}

func TestSetToneClampsTheEnsembleToItsCeiling(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: time.Minute})
	osc.SetTone(Tone{Partials: 8, CutoffHz: 1500, Voices: ensembleMax + 5, Cents: 7})

	if osc.voices != ensembleMax {
		t.Errorf("voices = %d, want the %d-copy ceiling", osc.voices, ensembleMax)
	}
}

func TestSingleCopyEnsembleTakesNoDetuneOffset(t *testing.T) {
	if got := ensembleOffset(0, 1, 40); got != 0 {
		t.Errorf("ensembleOffset(0, 1, 40) = %v, want 0: one copy has nothing to detune against", got)
	}
	if got := ensembleOffset(0, 2, 40); got != -20 {
		t.Errorf("ensembleOffset(0, 2, 40) = %v, want -20", got)
	}
	if got := ensembleOffset(1, 2, 40); got != 20 {
		t.Errorf("ensembleOffset(1, 2, 40) = %v, want 20", got)
	}
}

func TestBrightnessIsClampedBelowNyquist(t *testing.T) {
	f := Format{SampleRate: 8000, Channels: 2}
	osc := NewOsc(f, 220, 0.5, Envelope{Attack: 0, Release: time.Minute})
	osc.SetTone(Tone{Partials: 8, CutoffHz: 20000, Voices: 2, Cents: 7})

	ceiling := 1 - math.Exp(-twoPi*filterCeiling)
	if math.Abs(osc.lpA-ceiling) > 1e-12 {
		t.Errorf("filter coefficient = %v, want the Nyquist ceiling %v", osc.lpA, ceiling)
	}

	left, _ := channels(osc, 4096)
	if rms(left) == 0 {
		t.Error("a clamped filter produced silence, want audio")
	}
}

func TestDriftKeepsTheEnsembleWithinItsDetuneWindow(t *testing.T) {
	f := DefaultFormat()
	osc := NewOsc(f, 220, 0.5, Envelope{Attack: 0, Release: time.Minute})
	osc.SetTone(Tone{Partials: 12, CutoffHz: 1500, Voices: 3, Cents: 8, DriftHz: 2})

	widest := 8.0 * (0.5 + driftDepth)
	for range 32 {
		channels(osc, f.SampleRate/4)
		for i := range osc.voices {
			cents := 1200 * math.Log2(osc.ens[i].tgtL/220)
			if math.Abs(cents) > widest {
				t.Fatalf("copy %d drifted to %+.2f cents, want within %.2f", i, cents, widest)
			}
		}
	}
}

func TestStringsMixDoesNotAllocate(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("v", DroneBus, stringsOsc(t, f, 220, 0.5, harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1}))
	p := make([]byte, 1024*frameSize)

	allocs := testing.AllocsPerRun(100, func() {
		m.Read(p)
	})
	if allocs != 0 {
		t.Errorf("Read allocates %.0f allocs/run against a strings voice, want 0", allocs)
	}
}
