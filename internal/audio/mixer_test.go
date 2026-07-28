package audio

import (
	"math"
	"runtime"
	"sync"
	"testing"
	"time"
)

func decodeFrames(p []byte) [][2]float64 {
	frames := len(p) / frameSize
	out := make([][2]float64, frames)
	for i := range out {
		lb := uint32(p[i*8+0]) | uint32(p[i*8+1])<<8 | uint32(p[i*8+2])<<16 | uint32(p[i*8+3])<<24
		rb := uint32(p[i*8+4]) | uint32(p[i*8+5])<<8 | uint32(p[i*8+6])<<16 | uint32(p[i*8+7])<<24
		out[i][0] = float64(math.Float32frombits(lb))
		out[i][1] = float64(math.Float32frombits(rb))
	}
	return out
}

type constSource struct {
	l, r float32
}

func (s *constSource) Mix(buf [][2]float32) bool {
	for i := range buf {
		buf[i][0] += s.l
		buf[i][1] += s.r
	}
	return false
}

type countSource struct {
	val   float32
	limit int
	calls int
}

func (s *countSource) Mix(buf [][2]float32) bool {
	for i := range buf {
		buf[i][0] += s.val
		buf[i][1] += s.val
	}
	s.calls++
	return s.calls >= s.limit
}

func TestMixerKnownSum(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 0.4, r: 0.4})
	m.Add("b", &constSource{l: 0.4, r: 0.4})

	p := make([]byte, 256*frameSize)
	m.Read(p)
	frames := decodeFrames(p)

	wantL := math.Tanh((0.4 + 0.4) / 2.0)
	for i, fr := range frames {
		if math.Abs(fr[0]-wantL) > 1e-5 {
			t.Errorf("frame %d L = %.6f, want %.6f", i, fr[0], wantL)
			break
		}
	}
}

func TestMixerGainApplied(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 0.5, r: 0.5})
	m.SetGain(0.5)

	p := make([]byte, 64*frameSize)
	m.Read(p)
	frames := decodeFrames(p)

	want := math.Tanh(0.5 * 0.5)
	for _, fr := range frames {
		if math.Abs(fr[0]-want) > 1e-5 {
			t.Errorf("L = %.6f, want %.6f", fr[0], want)
			break
		}
	}
}

func TestMixerSoftClip(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	for i := 0; i < 12; i++ {
		id := string([]byte{byte('a' + i)})
		m.Add(id, &constSource{l: 1.0, r: 1.0})
	}

	p := make([]byte, 256*frameSize)
	m.Read(p)
	frames := decodeFrames(p)

	for i, fr := range frames {
		if fr[0] < -1.0 || fr[0] > 1.0 || fr[1] < -1.0 || fr[1] > 1.0 {
			t.Errorf("frame %d out of [-1,1]: L=%.4f R=%.4f", i, fr[0], fr[1])
			break
		}
	}
}

func TestMixerDoneSourceRemoved(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("v", &countSource{val: 0.3, limit: 1})

	p := make([]byte, 128*frameSize)
	m.Read(p)

	if m.Has("v") {
		t.Error("done source still present after Read")
	}
}

func TestMixerNaNGainRejected(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.SetGain(0.5)
	m.SetGain(math.NaN())
	if g := m.Gain(); g != 0.5 {
		t.Errorf("Gain() = %v after SetGain(NaN), want 0.5", g)
	}
}

func TestMixerPartialFrame(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 0.3, r: 0.3})

	p := make([]byte, 17)
	n, err := m.Read(p)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if n != 17 {
		t.Errorf("Read returned %d, want 17", n)
	}
}

func TestMixerRace(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	p := make([]byte, 512*frameSize)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			m.Read(p)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})
			m.Add("v", osc)
			runtime.Gosched()
			m.Remove("v")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			m.SetGain(0.5)
			runtime.Gosched()
			m.SetGain(1.0)
		}
	}()

	wg.Wait()
}

func TestMixerReadZeroAlloc(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})
	m.Add("v", osc)
	p := make([]byte, 1024*frameSize)

	allocs := testing.AllocsPerRun(100, func() {
		m.Read(p)
	})
	if allocs != 0 {
		t.Errorf("Read allocates %.0f allocs/run, want 0", allocs)
	}
}

func TestMixerEmptyRead(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	p := make([]byte, 256*frameSize)
	n, err := m.Read(p)
	if err != nil {
		t.Fatalf("Read on empty mixer: %v", err)
	}
	if n != len(p) {
		t.Errorf("n = %d, want %d", n, len(p))
	}
	for i, b := range p {
		if b != 0 {
			t.Errorf("byte %d = %d, want 0 (silence)", i, b)
			break
		}
	}
}
