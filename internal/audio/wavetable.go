package audio

import (
	"math"
	"sync"
)

const (
	tableSize      = 2048
	tableBands     = 11
	tableBaseHz    = 16.0
	tableBandLimit = 0.45
)

type tableKey struct {
	sampleRate int
	partials   int
}

type wavetable struct {
	bands [tableBands][]float64
	scale float64
}

var (
	tableMu    sync.Mutex
	tableCache = map[tableKey]*wavetable{}
)

func bandPartials(sampleRate, partials, band int) int {
	top := tableBaseHz * math.Exp2(float64(band+1))
	count := int(tableBandLimit * float64(sampleRate) / top)
	if count > partials {
		count = partials
	}
	if count < 1 {
		count = 1
	}
	return count
}

func buildWavetable(sampleRate, partials int) *wavetable {
	w := &wavetable{scale: tableSize / twoPi}
	for band := range w.bands {
		samples := make([]float64, tableSize+1)
		for n := 1; n <= bandPartials(sampleRate, partials, band); n++ {
			amp := 1 / float64(n)
			step := twoPi * float64(n) / tableSize
			for i := range tableSize {
				samples[i] += amp * math.Sin(step*float64(i))
			}
		}
		samples[tableSize] = samples[0]
		w.bands[band] = samples
	}
	norm := stackNorm(w.bands[0])
	for _, samples := range w.bands {
		for i := range samples {
			samples[i] *= norm
		}
	}
	return w
}

func stackNorm(samples []float64) float64 {
	sum := 0.0
	for _, s := range samples[:tableSize] {
		sum += s * s
	}
	return invSqrt2 / math.Sqrt(sum/tableSize)
}

func loadWavetable(sampleRate, partials int) *wavetable {
	key := tableKey{sampleRate: sampleRate, partials: partials}
	tableMu.Lock()
	defer tableMu.Unlock()
	if w, ok := tableCache[key]; ok {
		return w
	}
	w := buildWavetable(sampleRate, partials)
	tableCache[key] = w
	return w
}

func tableBand(freq float64) int {
	_, exp := math.Frexp(freq / tableBaseHz)
	band := exp - 1
	if band < 0 {
		return 0
	}
	if band >= tableBands {
		return tableBands - 1
	}
	return band
}

func (w *wavetable) at(band int, phase float64) float64 {
	pos := phase * w.scale
	i := int(pos)
	samples := w.bands[band]
	return samples[i] + (pos-float64(i))*(samples[i+1]-samples[i])
}
