package config

import (
	"errors"
	"fmt"

	"github.com/mach4-braai/hum/internal/harmony"
)

type Config struct {
	Project ProjectConfig `yaml:"project"`
	Music   MusicConfig   `yaml:"music"`
	Audio   AudioConfig   `yaml:"audio"`
}

type ProjectConfig struct {
	Name string `yaml:"name"`
}

type MusicConfig struct {
	Root  string `yaml:"root"`
	Scale string `yaml:"scale"`
	Theme string `yaml:"theme"`
}

type AudioConfig struct {
	Volume float64 `yaml:"volume"`
	Muted  bool    `yaml:"muted"`
}

func Default() Config {
	return Config{
		Music: MusicConfig{
			Root:  "D",
			Scale: "minor_pentatonic",
			Theme: "minimal",
		},
		Audio: AudioConfig{
			Volume: 0.6,
		},
	}
}

func (c Config) Validate() error {
	if c.Music.Root == "" {
		return errors.New("music.root: must not be empty")
	}
	if _, err := harmony.ParseNoteClass(c.Music.Root); err != nil {
		return fmt.Errorf("music.root: %w", err)
	}
	if c.Music.Scale == "" {
		return errors.New("music.scale: must not be empty")
	}
	if _, err := harmony.LookupScale(c.Music.Scale); err != nil {
		return fmt.Errorf("music.scale: %w", err)
	}
	if c.Music.Theme == "" {
		return errors.New("music.theme: must not be empty")
	}
	v := c.Audio.Volume
	if !(v >= 0 && v <= 1) {
		return fmt.Errorf("audio.volume: %v out of range [0, 1]", v)
	}
	return nil
}
