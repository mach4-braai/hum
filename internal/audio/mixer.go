package audio

import (
	"math"
	"sync"
)

const (
	maxScratchFrames = 4096
	maxSources       = 32
	frameSize        = 8
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

type Mixer struct {
	mu      sync.Mutex
	sources map[string]sourceEntry
	gen     uint64
	gain    float64

	scratch [][2]float32
	active  []sourceEntry
	done    []doneSource
}

func NewMixer(f Format) *Mixer {
	_ = f
	return &Mixer{
		sources: make(map[string]sourceEntry),
		gain:    1.0,
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

func (m *Mixer) Read(p []byte) (int, error) {
	frames := len(p) / frameSize

	m.mu.Lock()
	m.active = m.active[:0]
	for _, entry := range m.sources {
		m.active = append(m.active, entry)
	}
	voiceCount := len(m.active)
	gain := m.gain
	m.mu.Unlock()

	m.done = m.done[:0]

	norm := gain
	if voiceCount > 1 {
		norm /= float64(voiceCount)
	}

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
			}
		}
		for i, fr := range sc {
			l := math.Tanh(float64(fr[0]) * norm)
			r := math.Tanh(float64(fr[1]) * norm)
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
