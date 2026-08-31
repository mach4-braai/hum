package audio

import (
	"math"
	"sync"
)

const (
	maxScratchFrames = 4096
	maxSources       = 32
	frameSize        = 8

	rampTimeConstant = 0.040
	rampSettle       = 1e-9
)

type Source interface {
	Mix(buf [][2]float32) (done bool)
}

type sourceEntry struct {
	id  string
	gen uint64
	src Source
}

type doneSource struct {
	id  string
	gen uint64
}

type normRamp struct {
	current     float64
	target      float64
	coeff       float64
	initialized bool
}

func newNormRamp(sampleRate int) normRamp {
	return normRamp{coeff: math.Exp(-1.0 / (float64(sampleRate) * rampTimeConstant))}
}

func (r *normRamp) set(target float64) {
	if !r.initialized {
		r.current = target
		r.initialized = true
	}
	r.target = target
}

func (r *normRamp) step() float64 {
	if math.Abs(r.target-r.current) < rampSettle {
		r.current = r.target
		return r.current
	}
	r.current = r.coeff*r.current + (1-r.coeff)*r.target
	return r.current
}

type Mixer struct {
	mu      sync.Mutex
	sources map[string]sourceEntry
	gen     uint64
	gain    float64
	ramp    normRamp

	scratch [][2]float32
	active  []sourceEntry
	done    []doneSource
}

func NewMixer(f Format) *Mixer {
	return &Mixer{
		sources: make(map[string]sourceEntry),
		gain:    1.0,
		ramp:    newNormRamp(f.SampleRate),
		scratch: make([][2]float32, maxScratchFrames),
		active:  make([]sourceEntry, 0, maxSources),
		done:    make([]doneSource, 0, maxSources),
	}
}

func (m *Mixer) Add(id string, s Source) {
	m.mu.Lock()
	m.gen++
	m.sources[id] = sourceEntry{id: id, gen: m.gen, src: s}
	m.mu.Unlock()
}

func (m *Mixer) Remove(id string) {
	m.mu.Lock()
	delete(m.sources, id)
	m.mu.Unlock()
}

func (m *Mixer) Has(id string) bool {
	m.mu.Lock()
	_, ok := m.sources[id]
	m.mu.Unlock()
	return ok
}

func (m *Mixer) Len() int {
	m.mu.Lock()
	n := len(m.sources)
	m.mu.Unlock()
	return n
}

func (m *Mixer) SetGain(g float64) {
	if !(g >= 0 && g <= 1) {
		return
	}
	m.mu.Lock()
	m.gain = g
	m.mu.Unlock()
}

func (m *Mixer) Gain() float64 {
	m.mu.Lock()
	g := m.gain
	m.mu.Unlock()
	return g
}

func normTarget(voices int, gain float64) float64 {
	if voices <= 1 {
		return gain
	}
	return gain / float64(voices)
}

func (m *Mixer) Read(p []byte) (int, error) {
	frames := len(p) / frameSize

	m.mu.Lock()
	m.active = m.active[:0]
	for _, entry := range m.sources {
		m.active = append(m.active, entry)
	}
	gain := m.gain
	m.mu.Unlock()

	m.done = m.done[:0]

	alive := len(m.active)
	m.ramp.set(normTarget(alive, gain))

	written := 0
	remaining := frames
	for remaining > 0 {
		batch := remaining
		if batch > maxScratchFrames {
			batch = maxScratchFrames
		}
		sc := m.scratch[:batch]
		for i := range sc {
			sc[i] = [2]float32{}
		}
		for idx := range m.active {
			if m.active[idx].src == nil {
				continue
			}
			if m.active[idx].src.Mix(sc) {
				m.done = append(m.done, doneSource{id: m.active[idx].id, gen: m.active[idx].gen})
				m.active[idx].src = nil
				alive--
				m.ramp.set(normTarget(alive, gain))
			}
		}
		for i, fr := range sc {
			n := m.ramp.step()
			l := math.Tanh(float64(fr[0]) * n)
			r := math.Tanh(float64(fr[1]) * n)
			base := written + i*frameSize
			bl := math.Float32bits(float32(l))
			br := math.Float32bits(float32(r))
			p[base+0] = byte(bl)
			p[base+1] = byte(bl >> 8)
			p[base+2] = byte(bl >> 16)
			p[base+3] = byte(bl >> 24)
			p[base+4] = byte(br)
			p[base+5] = byte(br >> 8)
			p[base+6] = byte(br >> 16)
			p[base+7] = byte(br >> 24)
		}
		written += batch * frameSize
		remaining -= batch
	}

	if len(m.done) > 0 {
		m.mu.Lock()
		for _, finished := range m.done {
			if current, ok := m.sources[finished.id]; ok && current.gen == finished.gen {
				delete(m.sources, finished.id)
			}
		}
		m.mu.Unlock()
	}

	for i := written; i < len(p); i++ {
		p[i] = 0
	}

	return len(p), nil
}
