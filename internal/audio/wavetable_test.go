package audio

import (
	"math"
	"testing"
)

func TestBandPartialsFallWithPitchAndNeverReachZero(t *testing.T) {
	if got := bandPartials(48000, 12, 0); got != 12 {
		t.Errorf("bandPartials(48000, 12, 0) = %d, want the full stack of 12", got)
	}

	low := bandPartials(48000, 32, 5)
	high := bandPartials(48000, 32, 9)
	if high >= low {
		t.Errorf("band 9 holds %d partials against band 5's %d, want fewer as pitch rises", high, low)
	}

	if got := bandPartials(8000, 32, tableBands-1); got != 1 {
		t.Errorf("bandPartials(8000, 32, %d) = %d, want the fundamental alone", tableBands-1, got)
	}
}

func TestTableBandClampsBothEnds(t *testing.T) {
	if got := tableBand(1); got != 0 {
		t.Errorf("tableBand(1) = %d, want the lowest band", got)
	}
	if got := tableBand(tableBaseHz * math.Exp2(tableBands+3)); got != tableBands-1 {
		t.Errorf("tableBand above the top octave = %d, want %d", got, tableBands-1)
	}
	if got := tableBand(tableBaseHz); got != 0 {
		t.Errorf("tableBand(%v) = %d, want band 0", tableBaseHz, got)
	}
	if got := tableBand(2 * tableBaseHz); got != 1 {
		t.Errorf("tableBand(%v) = %d, want band 1", 2*tableBaseHz, got)
	}
}

func TestWavetableIsBuiltOncePerSampleRateAndStack(t *testing.T) {
	first := loadWavetable(48000, 12)
	again := loadWavetable(48000, 12)
	if first != again {
		t.Error("loadWavetable built a second table for the same sample rate and stack")
	}
	if other := loadWavetable(48000, 11); other == first {
		t.Error("loadWavetable reused a 12-partial table for an 11-partial stack")
	}
	if other := loadWavetable(44100, 12); other == first {
		t.Error("loadWavetable reused a 48 kHz table at 44.1 kHz")
	}
}

func TestWavetableCarriesTheEnergyOfAUnitSine(t *testing.T) {
	w := loadWavetable(48000, 12)
	band := w.bands[0]

	sum := 0.0
	for _, s := range band[:tableSize] {
		sum += s * s
	}
	got := math.Sqrt(sum / tableSize)
	if math.Abs(got-invSqrt2) > 1e-9 {
		t.Errorf("band 0 rms = %v, want a unit sine's %v", got, invSqrt2)
	}

	if band[tableSize] != band[0] {
		t.Errorf("guard sample = %v, want the wrap value %v", band[tableSize], band[0])
	}
}

func TestWavetableInterpolatesBetweenNeighbours(t *testing.T) {
	w := loadWavetable(48000, 12)
	band := w.bands[3]

	phase := 1.5 / w.scale
	got := w.at(3, phase)
	want := (band[1] + band[2]) / 2
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("at(3, %v) = %v, want the midpoint %v", phase, got, want)
	}
}
