package audio

import (
	"fmt"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

const benchVoices = 12

func benchOsc(b *testing.B, name string, freq float64) *Osc {
	b.Helper()
	b.Setenv("HUM_HOME", b.TempDir())
	th, err := theme.Load(name)
	if err != nil {
		b.Fatalf("load theme %q: %v", name, err)
	}
	f := DefaultFormat()
	osc := NewOsc(f, freq, 0.5, Envelope{Attack: 0, Release: time.Hour})
	osc.SetTone(ToneOf(th))
	osc.SetExpression(harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1}, th.Drone)
	return osc
}

func BenchmarkOscMix(b *testing.B) {
	for _, name := range []string{"minimal", "orchestra"} {
		b.Run(name, func(b *testing.B) {
			osc := benchOsc(b, name, 220)
			buf := make([][2]float32, 1024)
			b.SetBytes(int64(len(buf) * frameSize))
			b.ResetTimer()
			for b.Loop() {
				osc.Mix(buf)
			}
		})
	}
}

func BenchmarkMixerRead(b *testing.B) {
	for _, name := range []string{"minimal", "orchestra"} {
		b.Run(name, func(b *testing.B) {
			f := DefaultFormat()
			m := NewMixer(f)
			for voice := range benchVoices {
				m.Add(fmt.Sprintf("drone/%d", voice), benchOsc(b, name, 110*float64(1+voice)))
			}
			p := make([]byte, 1024*frameSize)
			b.SetBytes(int64(len(p)))
			b.ResetTimer()
			for b.Loop() {
				m.Read(p)
			}
		})
	}
}
