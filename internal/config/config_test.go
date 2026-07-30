package config

import (
	"math"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/harmony"
)

func TestDefaultPassesValidate(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v, want nil", err)
	}
}

func TestOctaveBoundsKeepEveryHarmonyInsideMidi(t *testing.T) {
	scale := harmony.Scale{Intervals: []int{0}}
	for _, octave := range []int{MinOctave, MaxOctave} {
		c := Default()
		c.Music.Octave = octave
		if err := c.Validate(); err != nil {
			t.Fatalf("octave %d rejected: %v", octave, err)
		}
		for class := range 12 {
			root := harmony.Pitch{Class: class, Octave: octave}
			ceiling := scale.Degree(root, 0).Transpose(24)
			if root.Midi() < harmony.MinMidi || ceiling.Midi() > harmony.MaxMidi {
				t.Errorf("octave %d class %d spans midi %d..%d, want inside [%d, %d]",
					octave, class, root.Midi(), ceiling.Midi(), harmony.MinMidi, harmony.MaxMidi)
			}
		}
	}
}

func TestValidateFieldErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "empty root",
			mutate: func(c *Config) { c.Music.Root = "" },
			want:   "music.root",
		},
		{
			name:   "empty scale",
			mutate: func(c *Config) { c.Music.Scale = "" },
			want:   "music.scale",
		},
		{
			name:   "empty theme",
			mutate: func(c *Config) { c.Music.Theme = "" },
			want:   "music.theme",
		},
		{
			name:   "root is not a note class",
			mutate: func(c *Config) { c.Music.Root = "H" },
			want:   "music.root",
		},
		{
			name:   "root carries an octave",
			mutate: func(c *Config) { c.Music.Root = "D2" },
			want:   "music.root",
		},
		{
			name:   "octave below the floor",
			mutate: func(c *Config) { c.Music.Octave = MinOctave - 1 },
			want:   "music.octave",
		},
		{
			name:   "octave above the ceiling",
			mutate: func(c *Config) { c.Music.Octave = MaxOctave + 1 },
			want:   "music.octave",
		},
		{
			name:   "scale is not a built-in",
			mutate: func(c *Config) { c.Music.Scale = "klingon" },
			want:   "music.scale",
		},
		{
			name:   "volume above 1",
			mutate: func(c *Config) { c.Audio.Volume = 1.7 },
			want:   "audio.volume",
		},
		{
			name:   "volume below 0",
			mutate: func(c *Config) { c.Audio.Volume = -0.1 },
			want:   "audio.volume",
		},
		{
			name:   "volume NaN",
			mutate: func(c *Config) { c.Audio.Volume = math.NaN() },
			want:   "audio.volume",
		},
		{
			name:   "volume +Inf",
			mutate: func(c *Config) { c.Audio.Volume = math.Inf(1) },
			want:   "audio.volume",
		},
		{
			name:   "volume -Inf",
			mutate: func(c *Config) { c.Audio.Volume = math.Inf(-1) },
			want:   "audio.volume",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			tc.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateVolumeEdges(t *testing.T) {
	c := Default()
	c.Audio.Volume = 0.0
	if err := c.Validate(); err != nil {
		t.Errorf("volume 0.0 should pass: %v", err)
	}
	c.Audio.Volume = 1.0
	if err := c.Validate(); err != nil {
		t.Errorf("volume 1.0 should pass: %v", err)
	}
}
