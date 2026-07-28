package theme

import (
	"fmt"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
)

type Theme struct {
	Name     string      `yaml:"name"`
	Waveform string      `yaml:"waveform"`
	Drone    DroneSpec   `yaml:"drone"`
	Phrases  PhrasesSpec `yaml:"phrases"`
}

type DroneSpec struct {
	Attack      float64 `yaml:"attack"`
	Release     float64 `yaml:"release"`
	Gain        float64 `yaml:"gain"`
	Harmonic    float64 `yaml:"harmonic"`
	TremoloHz   float64 `yaml:"tremolo_hz"`
	DetuneCents float64 `yaml:"detune_cents"`
}

type PhrasesSpec struct {
	CompletionOctaves  int     `yaml:"completion_octaves"`
	CompletionDuration float64 `yaml:"completion_duration"`
	CompletionGain     float64 `yaml:"completion_gain"`
	FailureInterval    int     `yaml:"failure_interval"`
	FailureDuration    float64 `yaml:"failure_duration"`
	FailureGain        float64 `yaml:"failure_gain"`
	CancelledSounds    bool    `yaml:"cancelled_sounds"`
	Attack             float64 `yaml:"attack"`
	Decay              float64 `yaml:"decay"`
}

func (t Theme) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("theme name must not be empty")
	}
	if t.Waveform != "sine" {
		return fmt.Errorf("waveform %q is not supported; only \"sine\" is valid in this release", t.Waveform)
	}
	if !(t.Drone.Attack > 0) {
		return fmt.Errorf("drone.attack must be positive, got %v", t.Drone.Attack)
	}
	if !(t.Drone.Release > 0) {
		return fmt.Errorf("drone.release must be positive, got %v", t.Drone.Release)
	}
	if !(t.Drone.Gain >= 0 && t.Drone.Gain <= 1) {
		return fmt.Errorf("drone.gain must be in [0,1], got %v", t.Drone.Gain)
	}
	if !(t.Drone.Harmonic >= 0 && t.Drone.Harmonic <= 1) {
		return fmt.Errorf("drone.harmonic must be in [0,1], got %v", t.Drone.Harmonic)
	}
	if !(t.Drone.TremoloHz >= 0) {
		return fmt.Errorf("drone.tremolo_hz must be non-negative, got %v", t.Drone.TremoloHz)
	}
	if t.Phrases.CompletionOctaves < 1 || t.Phrases.CompletionOctaves > 8 {
		return fmt.Errorf("phrases.completion_octaves must be in [1,8], got %d", t.Phrases.CompletionOctaves)
	}
	if !(t.Phrases.CompletionGain >= 0 && t.Phrases.CompletionGain <= 1) {
		return fmt.Errorf("phrases.completion_gain must be in [0,1], got %v", t.Phrases.CompletionGain)
	}
	if !(t.Phrases.CompletionDuration > 0) {
		return fmt.Errorf("phrases.completion_duration must be positive, got %v", t.Phrases.CompletionDuration)
	}
	if t.Phrases.FailureInterval >= 0 {
		return fmt.Errorf("phrases.failure_interval must be negative (descending), got %d", t.Phrases.FailureInterval)
	}
	if !(t.Phrases.FailureGain >= 0 && t.Phrases.FailureGain <= 1) {
		return fmt.Errorf("phrases.failure_gain must be in [0,1], got %v", t.Phrases.FailureGain)
	}
	if !(t.Phrases.FailureDuration > 0) {
		return fmt.Errorf("phrases.failure_duration must be positive, got %v", t.Phrases.FailureDuration)
	}
	if !(t.Phrases.Attack >= 0) {
		return fmt.Errorf("phrases.attack must be non-negative, got %v", t.Phrases.Attack)
	}
	if !(t.Phrases.Decay >= 0) {
		return fmt.Errorf("phrases.decay must be non-negative, got %v", t.Phrases.Decay)
	}
	return nil
}

func (t Theme) PhraseSpec() harmony.PhraseSpec {
	return harmony.PhraseSpec{
		CompletionOctaves:  t.Phrases.CompletionOctaves,
		CompletionDuration: time.Duration(t.Phrases.CompletionDuration * float64(time.Second)),
		CompletionGain:     t.Phrases.CompletionGain,
		FailureInterval:    t.Phrases.FailureInterval,
		FailureDuration:    time.Duration(t.Phrases.FailureDuration * float64(time.Second)),
		FailureGain:        t.Phrases.FailureGain,
		CancelledSounds:    t.Phrases.CancelledSounds,
	}
}
