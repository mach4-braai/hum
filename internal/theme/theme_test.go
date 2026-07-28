package theme

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMinimalEmbedded(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	th, err := Load("minimal")
	if err != nil {
		t.Fatalf("Load(\"minimal\") error: %v", err)
	}
	if th.Name != "minimal" {
		t.Errorf("Name = %q, want \"minimal\"", th.Name)
	}
	if th.Waveform != "sine" {
		t.Errorf("Waveform = %q, want \"sine\"", th.Waveform)
	}
}

func TestLoadShadowsEmbedded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	override := `
name: minimal
waveform: sine
drone:
  attack: 9.9
  release: 9.9
  gain: 0.9
  harmonic: 0.1
  tremolo_hz: 1.0
  detune_cents: 0.0
phrases:
  completion_octaves: 1
  completion_duration: 0.5
  completion_gain: 0.5
  failure_interval: -1
  failure_duration: 0.5
  failure_gain: 0.5
  cancelled_sounds: false
  attack: 0.01
  decay: 0.1
`
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), []byte(override), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	th, err := Load("minimal")
	if err != nil {
		t.Fatalf("Load shadow: %v", err)
	}
	if th.Drone.Attack != 9.9 {
		t.Errorf("shadow not active: drone.attack = %v, want 9.9", th.Drone.Attack)
	}
}

func TestLoadUnknownTheme(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown theme")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q does not mention theme name", err.Error())
	}
	if !strings.Contains(err.Error(), "minimal") {
		t.Errorf("error %q does not list available themes", err.Error())
	}
}

func TestLoadUserThemeUnknownField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bad := `
name: minimal
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 0.5
  harmonic: 0.1
  tremolo_hz: 5.0
  detune_cents: 8.0
  unknown_field: oops
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
`
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load("minimal")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Errorf("error %q does not name unknown field", err.Error())
	}
}

func TestLoadUserThemeInvalidValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bad := `
name: minimal
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 2.0
  harmonic: 0.1
  tremolo_hz: 5.0
  detune_cents: 8.0
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
`
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load("minimal")
	if err == nil {
		t.Fatal("expected error for invalid drone.gain")
	}
	if !strings.Contains(err.Error(), "drone.gain") {
		t.Errorf("error %q does not name field", err.Error())
	}
}

func TestLoadUserThemePositiveFailureInterval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bad := `
name: minimal
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 0.5
  harmonic: 0.1
  tremolo_hz: 5.0
  detune_cents: 8.0
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: 3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
`
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Load("minimal")
	if err == nil {
		t.Fatal("expected error for non-negative failure_interval")
	}
	if !strings.Contains(err.Error(), "failure_interval") {
		t.Errorf("error %q does not name field", err.Error())
	}
}

func TestListSortedDeduplicated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	names := List()
	if len(names) == 0 {
		t.Fatal("List() returned no themes")
	}
	for i := 1; i < len(names); i++ {
		if names[i] <= names[i-1] {
			t.Errorf("List() not sorted: %v", names)
		}
	}
	seen := make(map[string]struct{})
	for _, n := range names {
		if _, dup := seen[n]; dup {
			t.Errorf("List() contains duplicate %q", n)
		}
		seen[n] = struct{}{}
	}
}

func TestListShadowedThemeOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), []byte("x: 1"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	names := List()
	count := 0
	for _, n := range names {
		if n == "minimal" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("shadowed built-in appears %d times in List(), want 1", count)
	}
}

func TestListMissingUserDirNotError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	names := List()
	if len(names) == 0 {
		t.Error("List() should still return embedded themes when user dir is missing")
	}
}

func TestEmbeddedMinimalPassesValidate(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	th, err := Load("minimal")
	if err != nil {
		t.Fatalf("embedded minimal failed Validate: %v", err)
	}
	if err := th.Validate(); err != nil {
		t.Fatalf("Validate on loaded minimal: %v", err)
	}
}

func TestPhraseSpecFromMinimal(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	th, err := Load("minimal")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ps := th.PhraseSpec()
	if ps.CompletionOctaves != 2 {
		t.Errorf("CompletionOctaves = %d, want 2", ps.CompletionOctaves)
	}
	if ps.FailureInterval != -3 {
		t.Errorf("FailureInterval = %d, want -3", ps.FailureInterval)
	}
	if ps.CancelledSounds {
		t.Error("CancelledSounds should be false")
	}
	wantCompletion := time.Duration(0.2 * float64(time.Second))
	if ps.CompletionDuration != wantCompletion {
		t.Errorf("CompletionDuration = %v, want %v", ps.CompletionDuration, wantCompletion)
	}
	wantFailure := time.Duration(1.2 * float64(time.Second))
	if ps.FailureDuration != wantFailure {
		t.Errorf("FailureDuration = %v, want %v", ps.FailureDuration, wantFailure)
	}
}

func TestValidateEmptyName(t *testing.T) {
	th := Theme{Waveform: "sine"}
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("expected name error, got %v", err)
	}
}

func TestValidateUnsupportedWaveform(t *testing.T) {
	th := Theme{Name: "t", Waveform: "square"}
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "waveform") {
		t.Errorf("expected waveform error, got %v", err)
	}
}

func TestValidateNonPositiveAttack(t *testing.T) {
	th := Theme{Name: "t", Waveform: "sine", Drone: DroneSpec{Attack: 0, Release: 1}}
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "attack") {
		t.Errorf("expected attack error, got %v", err)
	}
}

func TestValidateNonPositiveRelease(t *testing.T) {
	th := Theme{Name: "t", Waveform: "sine", Drone: DroneSpec{Attack: 1, Release: -1}}
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "release") {
		t.Errorf("expected release error, got %v", err)
	}
}

func TestValidateNaNGain(t *testing.T) {
	th := minimalValid()
	th.Drone.Gain = math.NaN()
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "drone.gain") {
		t.Errorf("expected gain NaN error, got %v", err)
	}
}

func TestValidateGainAboveOne(t *testing.T) {
	th := minimalValid()
	th.Drone.Gain = 1.1
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "drone.gain") {
		t.Errorf("expected gain error, got %v", err)
	}
}

func TestValidateHarmonicOutOfRange(t *testing.T) {
	th := minimalValid()
	th.Drone.Harmonic = -0.1
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "drone.harmonic") {
		t.Errorf("expected harmonic error, got %v", err)
	}
}

func TestValidateNegativeTremoloHz(t *testing.T) {
	th := minimalValid()
	th.Drone.TremoloHz = -1
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "tremolo_hz") {
		t.Errorf("expected tremolo_hz error, got %v", err)
	}
}

func TestValidateCompletionOctavesZero(t *testing.T) {
	th := minimalValid()
	th.Phrases.CompletionOctaves = 0
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "completion_octaves") {
		t.Errorf("expected completion_octaves error, got %v", err)
	}
}

func TestValidateCompletionOctavesTooHigh(t *testing.T) {
	th := minimalValid()
	th.Phrases.CompletionOctaves = 9
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "completion_octaves") {
		t.Errorf("expected completion_octaves error, got %v", err)
	}
}

func TestValidateCompletionGainNaN(t *testing.T) {
	th := minimalValid()
	th.Phrases.CompletionGain = math.NaN()
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "completion_gain") {
		t.Errorf("expected completion_gain error, got %v", err)
	}
}

func TestValidateCompletionDurationZero(t *testing.T) {
	th := minimalValid()
	th.Phrases.CompletionDuration = 0
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "completion_duration") {
		t.Errorf("expected completion_duration error, got %v", err)
	}
}

func TestValidateFailureIntervalZero(t *testing.T) {
	th := minimalValid()
	th.Phrases.FailureInterval = 0
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "failure_interval") {
		t.Errorf("expected failure_interval error, got %v", err)
	}
}

func TestValidateFailureGainNaN(t *testing.T) {
	th := minimalValid()
	th.Phrases.FailureGain = math.NaN()
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "failure_gain") {
		t.Errorf("expected failure_gain error, got %v", err)
	}
}

func TestValidateFailureDurationZero(t *testing.T) {
	th := minimalValid()
	th.Phrases.FailureDuration = 0
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "failure_duration") {
		t.Errorf("expected failure_duration error, got %v", err)
	}
}

func TestValidateNegativePhraseAttack(t *testing.T) {
	th := minimalValid()
	th.Phrases.Attack = -0.01
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "phrases.attack") {
		t.Errorf("expected phrases.attack error, got %v", err)
	}
}

func TestValidateNegativePhraseDecay(t *testing.T) {
	th := minimalValid()
	th.Phrases.Decay = -0.01
	err := th.Validate()
	if err == nil || !strings.Contains(err.Error(), "phrases.decay") {
		t.Errorf("expected phrases.decay error, got %v", err)
	}
}

func minimalValid() Theme {
	return Theme{
		Name:     "minimal",
		Waveform: "sine",
		Drone: DroneSpec{
			Attack:      2.5,
			Release:     3.0,
			Gain:        0.5,
			Harmonic:    0.15,
			TremoloHz:   5.0,
			DetuneCents: 8.0,
		},
		Phrases: PhrasesSpec{
			CompletionOctaves:  2,
			CompletionDuration: 0.2,
			CompletionGain:     0.7,
			FailureInterval:    -3,
			FailureDuration:    1.2,
			FailureGain:        0.35,
			CancelledSounds:    false,
			Attack:             0.02,
			Decay:              0.15,
		},
	}
}

func TestLoadUserThemeReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	path := filepath.Join(home, "themes", "minimal.yaml")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}

	_, err := Load("minimal")
	if err == nil {
		t.Fatal("expected error when user theme path is a directory")
	}
}

func TestDecodeYAMLBadSyntax(t *testing.T) {
	_, err := decodeYAML([]byte(": bad: yaml: :::"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestDecodeYAMLStrictRejectsUnknownField(t *testing.T) {
	data := []byte(`
name: minimal
waveform: sine
drone:
  attack: 2.5
  release: 3.0
  gain: 0.5
  harmonic: 0.15
  tremolo_hz: 5.0
  detune_cents: 8.0
  spurious_key: 999
phrases:
  completion_octaves: 2
  completion_duration: 0.2
  completion_gain: 0.7
  failure_interval: -3
  failure_duration: 1.2
  failure_gain: 0.35
  cancelled_sounds: false
  attack: 0.02
  decay: 0.15
`)
	_, err := decodeYAML(data)
	if err == nil {
		t.Fatal("expected error for unknown field in strict mode")
	}
	if !strings.Contains(err.Error(), "spurious_key") {
		t.Errorf("error %q does not name field", err.Error())
	}
}

func TestLoadRejectsAPathAsAThemeName(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	for _, name := range []string{"", ".", "..", "../minimal", "sub/minimal", "/etc/passwd"} {
		if _, err := Load(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Load(%q) error = %v, want ErrInvalidName", name, err)
		}
	}
}
