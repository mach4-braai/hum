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

func TestMixerSoftClipNeverExact(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 2.0, r: 2.0})

	p := make([]byte, 256*frameSize)
	m.Read(p)
	frames := decodeFrames(p)

	for i, fr := range frames {
		if fr[0] >= 1.0 || fr[1] >= 1.0 {
			t.Errorf("frame %d: hard clipping detected (L=%.8f R=%.8f); tanh(2.0)≈0.964 must be < 1.0", i, fr[0], fr[1])
			break
		}
		if fr[0] < 0.9 || fr[1] < 0.9 {
			t.Errorf("frame %d: output unexpectedly low (L=%.6f R=%.6f); expected tanh(2.0)≈0.964", i, fr[0], fr[1])
			break
		}
	}
}

func TestMixerZeroLengthRead(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 0.5, r: 0.5})

	p := make([]byte, 0)
	n, err := m.Read(p)
	if err != nil {
		t.Fatalf("zero-length Read: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if m.Len() != 1 {
		t.Error("zero-length Read must not remove sources")
	}
}

func TestMixerDoneMidBufferLenDrops(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &countSource{val: 0.3, limit: 1})
	m.Add("b", &constSource{l: 0.2, r: 0.2})

	if m.Len() != 2 {
		t.Fatalf("want 2 sources, got %d", m.Len())
	}

	p := make([]byte, 128*frameSize)
	m.Read(p)

	if m.Len() != 1 {
		t.Errorf("Len after done source: got %d, want 1", m.Len())
	}
	if m.Has("a") {
		t.Error("done source 'a' must be removed")
	}
	if !m.Has("b") {
		t.Error("surviving source 'b' must remain")
	}
}

type replacingSource struct {
	m        *Mixer
	id       string
	swapped  bool
	replaced Source
}

func (r *replacingSource) Mix(buf [][2]float32) bool {
	if !r.swapped {
		r.swapped = true
		r.m.Add(r.id, r.replaced)
	}
	return true
}

func TestReadKeepsASourceAddedUnderAFinishingID(t *testing.T) {
	m := NewMixer(DefaultFormat())

	replacement := &constSource{l: 0.25, r: 0.25}
	finishing := &replacingSource{m: m, id: "voice", replaced: replacement}
	m.Add("voice", finishing)

	p := make([]byte, 64*frameSize)
	if _, err := m.Read(p); err != nil {
		t.Fatalf("Read: %v", err)
	}

	if !m.Has("voice") {
		t.Fatal("the replacement added while the previous source finished was deleted; a session would start and never sound")
	}
	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1", m.Len())
	}

	if _, err := m.Read(p); err != nil {
		t.Fatalf("second Read: %v", err)
	}
	var nonZero bool
	for i := 0; i < len(p); i += frameSize {
		if p[i] != 0 || p[i+1] != 0 || p[i+2] != 0 || p[i+3] != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Error("the replacement produced silence, so it never reached the mix")
	}
}

func maxBufferDelta(frames [][2]float64, prevLast float64) float64 {
	var maxD float64
	if len(frames) == 0 {
		return 0
	}
	if d := math.Abs(frames[0][0] - prevLast); d > maxD {
		maxD = d
	}
	for i := 1; i < len(frames); i++ {
		if d := math.Abs(frames[i][0] - frames[i-1][0]); d > maxD {
			maxD = d
		}
	}
	return maxD
}

func TestNormRampNoStepOnVoiceAdd(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)

	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})
	m.Add("v1", osc)

	buf := make([]byte, 4096*frameSize)

	m.Read(buf)
	m.Read(buf)
	m.Read(buf)
	ss := decodeFrames(buf)
	var maxSS float64
	for i := 1; i < len(ss); i++ {
		if d := math.Abs(ss[i][0] - ss[i-1][0]); d > maxSS {
			maxSS = d
		}
	}
	lastSS := ss[len(ss)-1][0]

	m.Add("v2", &countSource{val: 0, limit: 999})

	m.Read(buf)
	tr := decodeFrames(buf)
	maxTr := maxBufferDelta(tr, lastSS)

	if maxTr > maxSS*2 {
		t.Errorf("voice add caused a step: %.6f > 2× steady-state %.6f; norm ramp did not smooth the transition",
			maxTr, maxSS)
	}
}

func TestNormRampNoStepOnVoiceRelease(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)

	osc := NewOsc(f, 440, 0.5, Envelope{Attack: 0, Release: 10 * time.Second})
	m.Add("v1", osc)

	buf := make([]byte, 4096*frameSize)

	m.Read(buf)
	m.Read(buf)
	m.Read(buf)
	ss := decodeFrames(buf)
	var maxSS float64
	for i := 1; i < len(ss); i++ {
		if d := math.Abs(ss[i][0] - ss[i-1][0]); d > maxSS {
			maxSS = d
		}
	}

	ghost := &countSource{val: 0, limit: 10}
	m.Add("ghost", ghost)

	for range 8 {
		m.Read(buf)
	}

	m.Read(buf)
	pre := decodeFrames(buf)
	lastPre := pre[len(pre)-1][0]

	m.Read(buf)
	dying := decodeFrames(buf)
	maxTr := maxBufferDelta(dying, lastPre)
	lastDying := dying[len(dying)-1][0]

	m.Read(buf)
	after := decodeFrames(buf)
	if d := maxBufferDelta(after, lastDying); d > maxTr {
		maxTr = d
	}

	if maxTr > maxSS*2 {
		t.Errorf("voice release caused a step: %.6f > 2× steady-state %.6f; norm ramp did not smooth the transition",
			maxTr, maxSS)
	}
}

func TestNormRampNoStepMidBufferDone(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)

	m.Add("a", &constSource{l: 0.5, r: 0.5})
	m.Add("ghost", &countSource{val: 0, limit: 1})

	buf := make([]byte, 2*maxScratchFrames*frameSize)
	m.Read(buf)
	frames := decodeFrames(buf)
	last := frames[len(frames)-1][0]

	want := math.Tanh(0.5 * 0.9)
	if last < want {
		t.Errorf("mid-buffer done did not update ramp target: last-frame level %.6f < %.6f; "+
			"alive and ramp target must update the sample the source reports done",
			last, want)
	}
}

func TestNormRampSteadyStateMatchesUnrampedCurve(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	m.Add("a", &constSource{l: 0.4, r: 0.4})

	large := make([]byte, 16384*frameSize)
	m.Read(large)

	m.Add("b", &constSource{l: 0.4, r: 0.4})

	m.Read(large)

	buf := make([]byte, 256*frameSize)
	m.Read(buf)
	frames := decodeFrames(buf)

	want := math.Tanh(0.8 * 0.5)
	for i, fr := range frames {
		if math.Abs(fr[0]-want) > 1e-3 {
			t.Errorf("frame %d: L=%.6f, want %.6f; steady-state differs from unramped 1/N curve after ramp settled",
				i, fr[0], want)
			break
		}
	}
}
