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

type Bus int

const (
	DroneBus Bus = iota
	PhraseBus
	busCount
)

type Source interface {
	Mix(buf [][2]float32) (done bool)
}

type sourceEntry struct {
	id  string
	gen uint64
	bus Bus
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

func (r *normRamp) snap() {
	r.current = r.target
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
	ramps   [busCount]normRamp

	scratch [busCount][][2]float32
	active  []sourceEntry
	done    []doneSource
	alive   [busCount]int
}

func NewMixer(f Format) *Mixer {
	m := &Mixer{
		sources: make(map[string]sourceEntry),
		gain:    1.0,
		active:  make([]sourceEntry, 0, maxSources),
		done:    make([]doneSource, 0, maxSources),
	}
	for bus := range m.scratch {
		m.ramps[bus] = newNormRamp(f.SampleRate)
		m.scratch[bus] = make([][2]float32, maxScratchFrames)
	}
	return m
}

func (m *Mixer) Add(id string, bus Bus, s Source) {
	m.mu.Lock()
	m.gen++
	m.sources[id] = sourceEntry{id: id, gen: m.gen, bus: bus, src: s}
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

func incoherentNorm(voices int, gain float64) float64 {
	if voices <= 1 {
		return gain
	}
	return gain / math.Sqrt(float64(voices))
}

func (m *Mixer) retarget(gain float64) {
	m.ramps[DroneBus].set(normTarget(m.alive[DroneBus], gain))
	m.ramps[PhraseBus].set(incoherentNorm(m.alive[PhraseBus], gain))
}

func putFrame(p []byte, base int, l, r float64) {
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

	m.alive = [busCount]int{}
	for idx := range m.active {
		m.alive[m.active[idx].bus]++
	}
	m.retarget(gain)

	written := 0
	remaining := frames
	for remaining > 0 {
		batch := remaining
		if batch > maxScratchFrames {
			batch = maxScratchFrames
		}

		sounding := m.alive[PhraseBus] > 0
		drone := m.scratch[DroneBus][:batch]
		phrase := m.scratch[PhraseBus][:batch]
		for i := range drone {
			drone[i] = [2]float32{}
		}
		if sounding {
			for i := range phrase {
				phrase[i] = [2]float32{}
			}
		} else {
			m.ramps[PhraseBus].snap()
		}

		for idx := range m.active {
			if m.active[idx].src == nil {
				continue
			}
			bus := m.active[idx].bus
			if m.active[idx].src.Mix(m.scratch[bus][:batch]) {
				m.done = append(m.done, doneSource{id: m.active[idx].id, gen: m.active[idx].gen})
				m.active[idx].src = nil
				m.alive[bus]--
				m.retarget(gain)
			}
		}

		for i := range batch {
			dn := m.ramps[DroneBus].step()
			l := float64(drone[i][0]) * dn
			r := float64(drone[i][1]) * dn
			if sounding {
				pn := m.ramps[PhraseBus].step()
				l += float64(phrase[i][0]) * pn
				r += float64(phrase[i][1]) * pn
			}
			putFrame(p, written+i*frameSize, math.Tanh(l), math.Tanh(r))
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
