package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestResolveDefaultsOnlyProvenance(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	def := Default()
	if c.Music.Root != def.Music.Root {
		t.Errorf("Root = %q, want %q", c.Music.Root, def.Music.Root)
	}
	if c.Audio.Volume != def.Audio.Volume {
		t.Errorf("Volume = %v, want %v", c.Audio.Volume, def.Audio.Volume)
	}

	for _, key := range []string{"project.name", "music.root", "music.scale", "music.theme", "audio.volume", "audio.muted", "session.max_lease"} {
		if prov[key] != LayerDefault {
			t.Errorf("prov[%s] = %q, want %q", key, prov[key], LayerDefault)
		}
	}
}

func TestResolveGlobalLayerApplied(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "music:\n  root: F\n  scale: dorian\n  theme: minimal\n")

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Music.Root != "F" {
		t.Errorf("Root = %q, want F", c.Music.Root)
	}
	if prov["music.root"] != LayerGlobal {
		t.Errorf("prov[music.root] = %q, want global", prov["music.root"])
	}
	if prov["audio.volume"] != LayerDefault {
		t.Errorf("prov[audio.volume] = %q, want default", prov["audio.volume"])
	}
}

func TestResolveAbsentProjectFallsToGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "music:\n  root: G\n  scale: major\n  theme: minimal\n")

	dir := filepath.Join(tmp, "noproj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Music.Root != "G" {
		t.Errorf("Root = %q, want G", c.Music.Root)
	}
	if prov["music.root"] != LayerGlobal {
		t.Errorf("prov[music.root] = %q, want global", prov["music.root"])
	}
}

func TestResolveProjectBeatsGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "music:\n  root: C\n  scale: major\n  theme: minimal\n")

	projDir := filepath.Join(tmp, "myproject")
	writeYAML(t, filepath.Join(projDir, ".hum"), "music:\n  root: E\n")

	c, prov, err := Resolve(nil, projDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Music.Root != "E" {
		t.Errorf("Root = %q, want E", c.Music.Root)
	}
	if prov["music.root"] != LayerProject {
		t.Errorf("prov[music.root] = %q, want project", prov["music.root"])
	}
	if prov["music.scale"] != LayerGlobal {
		t.Errorf("prov[music.scale] = %q, want global", prov["music.scale"])
	}
}

func TestResolveProjectVolumeZeroBeatsGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "music:\n  root: D\n  scale: minor_pentatonic\n  theme: minimal\naudio:\n  volume: 0.9\n")

	projDir := filepath.Join(tmp, "myproject")
	writeYAML(t, filepath.Join(projDir, ".hum"), "audio:\n  volume: 0\n")

	c, prov, err := Resolve(nil, projDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Audio.Volume != 0 {
		t.Errorf("Volume = %v, want 0", c.Audio.Volume)
	}
	if prov["audio.volume"] != LayerProject {
		t.Errorf("prov[audio.volume] = %q, want project", prov["audio.volume"])
	}
}

func TestResolveCLIWinsAllLayers(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "music:\n  root: C\n  scale: major\n  theme: minimal\naudio:\n  volume: 0.5\n")

	projDir := filepath.Join(tmp, "proj")
	writeYAML(t, filepath.Join(projDir, ".hum"), "music:\n  root: E\n")

	c, prov, err := Resolve(map[string]string{"music.root": "G#"}, projDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Music.Root != "G#" {
		t.Errorf("Root = %q, want G#", c.Music.Root)
	}
	if prov["music.root"] != LayerCLI {
		t.Errorf("prov[music.root] = %q, want cli", prov["music.root"])
	}
}

func TestResolveCLIAllFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	overrides := map[string]string{
		"project.name":      "myproject",
		"music.root":        "A",
		"music.octave":      "2",
		"music.scale":       "major",
		"music.theme":       "minimal",
		"audio.volume":      "0.3",
		"audio.muted":       "true",
		"session.max_lease": "1h",
	}
	c, prov, err := Resolve(overrides, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Project.Name != "myproject" {
		t.Errorf("Name = %q, want myproject", c.Project.Name)
	}
	if c.Music.Root != "A" {
		t.Errorf("Root = %q, want A", c.Music.Root)
	}
	if c.Music.Octave != 2 {
		t.Errorf("Octave = %d, want 2", c.Music.Octave)
	}
	if c.Audio.Volume != 0.3 {
		t.Errorf("Volume = %v, want 0.3", c.Audio.Volume)
	}
	if !c.Audio.Muted {
		t.Error("Muted = false, want true")
	}
	if c.Session.MaxLease != "1h" {
		t.Errorf("MaxLease = %q, want 1h", c.Session.MaxLease)
	}
	for _, key := range []string{"project.name", "music.root", "music.octave", "music.scale", "music.theme", "audio.volume", "audio.muted", "session.max_lease"} {
		if prov[key] != LayerCLI {
			t.Errorf("prov[%s] = %q, want cli", key, prov[key])
		}
	}
}

func TestResolveCLIInvalidOctave(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"music.octave": "low"}, dir)
	if err == nil {
		t.Fatal("expected error for a non-integer octave")
	}
	if !strings.Contains(err.Error(), "music.octave") {
		t.Errorf("error %q does not identify music.octave", err)
	}
}

func TestResolveCLIInvalidMaxLease(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"session.max_lease": "forever"}, dir)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
	if !strings.Contains(err.Error(), "session.max_lease") {
		t.Errorf("error %q does not identify session.max_lease", err)
	}
}

func TestResolveCLINegativeMaxLease(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"session.max_lease": "-1h"}, dir)
	if err == nil {
		t.Fatal("expected error for negative max_lease")
	}
	if !strings.Contains(err.Error(), "session.max_lease") {
		t.Errorf("error %q does not identify session.max_lease", err)
	}
}

func TestResolveOctaveFromProjectLayer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	projDir := filepath.Join(tmp, "myproject")
	writeYAML(t, filepath.Join(projDir, ".hum"), "music:\n  root: D\n  octave: 2\n")

	c, prov, err := Resolve(nil, projDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Music.Octave != 2 {
		t.Errorf("Octave = %d, want the project's 2", c.Music.Octave)
	}
	if prov["music.octave"] != LayerProject {
		t.Errorf("prov[music.octave] = %q, want project", prov["music.octave"])
	}
}

func TestResolveCLIUnknownKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"unknown.key": "val"}, dir)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown.key") {
		t.Errorf("error %q missing key name", err.Error())
	}
}

func TestResolveCLIInvalidVolume(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"audio.volume": "notanumber"}, dir)
	if err == nil {
		t.Fatal("expected error for invalid volume")
	}
	if !strings.Contains(err.Error(), "audio.volume") {
		t.Errorf("error %q missing field name", err.Error())
	}
}

func TestResolveCLIVolumeOutOfRange(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"audio.volume": "1.5"}, dir)
	if err == nil {
		t.Fatal("expected error for volume out of range")
	}
	if !strings.Contains(err.Error(), "audio.volume") {
		t.Errorf("error %q missing field name", err.Error())
	}
}

func TestResolveCLIVolumeNaN(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"audio.volume": "NaN"}, dir)
	if err == nil {
		t.Fatal("expected error for NaN volume")
	}
	if !strings.Contains(err.Error(), "audio.volume") {
		t.Errorf("error %q missing field name", err.Error())
	}
}

func TestResolveCLIInvalidMuted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"audio.muted": "maybe"}, dir)
	if err == nil {
		t.Fatal("expected error for invalid muted")
	}
	if !strings.Contains(err.Error(), "audio.muted") {
		t.Errorf("error %q missing field name", err.Error())
	}
}

func TestResolveValidationError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)
	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(map[string]string{"music.root": ""}, dir)
	if err == nil {
		t.Fatal("expected validation error for empty root")
	}
	if !strings.Contains(err.Error(), "music.root") {
		t.Errorf("error %q missing field name", err.Error())
	}
}

func TestResolveGlobalLoadError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	orig := osOpen
	t.Cleanup(func() { osOpen = orig })
	osOpen = func(name string) (*os.File, error) {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
	}

	dir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(nil, dir)
	if err == nil {
		t.Fatal("expected error for unreadable global config")
	}
}

func TestResolveProjectLoadError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	projDir := filepath.Join(tmp, "myproject")
	humDir := filepath.Join(projDir, ".hum")
	projCfg := filepath.Join(humDir, "config.yaml")
	writeYAML(t, humDir, "music:\n  root: D\n")

	orig := osOpen
	t.Cleanup(func() { osOpen = orig })
	osOpen = func(name string) (*os.File, error) {
		if name == projCfg {
			return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
		}
		return os.Open(name)
	}

	_, _, err := Resolve(nil, projDir)
	if err == nil {
		t.Fatal("expected error for unreadable project config")
	}
}

func TestResolveLayerUnknownField(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "theme: orchestra\nmusic:\n  root: D\n  scale: dorian\n  theme: minimal\n")

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(nil, dir)
	if err == nil {
		t.Fatal("expected error for unknown field in global config")
	}
}

func TestResolveAllFieldsFromGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "project:\n  name: myproject\nmusic:\n  root: A\n  scale: major\n  theme: minimal\naudio:\n  volume: 0.7\n  muted: true\n")

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Project.Name != "myproject" {
		t.Errorf("Name = %q, want myproject", c.Project.Name)
	}
	if !c.Audio.Muted {
		t.Error("Muted = false, want true")
	}
	if prov["project.name"] != LayerGlobal {
		t.Errorf("prov[project.name] = %q, want global", prov["project.name"])
	}
	if prov["audio.muted"] != LayerGlobal {
		t.Errorf("prov[audio.muted] = %q, want global", prov["audio.muted"])
	}
}

func TestResolveAllFieldsFromProject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	projDir := filepath.Join(tmp, "myproject")
	writeYAML(t, filepath.Join(projDir, ".hum"), "project:\n  name: fromproject\nmusic:\n  root: B\n  scale: dorian\n  theme: minimal\naudio:\n  volume: 0.4\n  muted: true\n")

	c, prov, err := Resolve(nil, projDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Project.Name != "fromproject" {
		t.Errorf("Name = %q, want fromproject", c.Project.Name)
	}
	if !c.Audio.Muted {
		t.Error("Muted = false, want true")
	}
	if prov["project.name"] != LayerProject {
		t.Errorf("prov[project.name] = %q, want project", prov["project.name"])
	}
	if prov["audio.muted"] != LayerProject {
		t.Errorf("prov[audio.muted] = %q, want project", prov["audio.muted"])
	}
}

func TestCanonicalRootFailsWhenEvalSymlinksFails(t *testing.T) {
	t.Cleanup(func() { evalSymlinks = filepath.EvalSymlinks })
	evalSymlinks = func(string) (string, error) { return "", errors.New("lstat: no such file") }

	dir := t.TempDir()
	_, err := CanonicalRoot(dir)
	if err == nil {
		t.Fatal("CanonicalRoot = nil, want error")
	}
	if !errors.Is(err, ErrProjectRoot) {
		t.Errorf("err = %v, want to wrap ErrProjectRoot", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%q", dir)) {
		t.Errorf("err = %v, want it to name the offending root %q; the message quotes the path, which doubles separators on Windows", err, dir)
	}
	if !strings.Contains(err.Error(), "lstat: no such file") {
		t.Errorf("err = %v, want the underlying resolver failure kept", err)
	}
}

func TestResolveSessionMaxLeaseFromGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "session:\n  max_lease: \"2h\"\n")

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Session.MaxLease != "2h" {
		t.Errorf("MaxLease = %q, want 2h", c.Session.MaxLease)
	}
	if prov["session.max_lease"] != LayerGlobal {
		t.Errorf("prov[session.max_lease] = %q, want global", prov["session.max_lease"])
	}
}

func TestResolveSessionMaxLeaseProjectOverridesGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "session:\n  max_lease: \"24h\"\n")

	project := filepath.Join(tmp, "project", ".hum")
	writeYAML(t, project, "session:\n  max_lease: \"1h\"\n")

	c, prov, err := Resolve(nil, filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Session.MaxLease != "1h" {
		t.Errorf("MaxLease = %q, want 1h", c.Session.MaxLease)
	}
	if prov["session.max_lease"] != LayerProject {
		t.Errorf("prov[session.max_lease] = %q, want project", prov["session.max_lease"])
	}
}

func TestResolveSessionMaxLeaseProjectClearsGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "session:\n  max_lease: \"24h\"\n")

	project := filepath.Join(tmp, "project", ".hum")
	writeYAML(t, project, "session:\n  max_lease: \"\"\n")

	c, _, err := Resolve(nil, filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Session.MaxLease != "" {
		t.Errorf("MaxLease = %q, want empty (project disabled the lease)", c.Session.MaxLease)
	}
}

func TestResolveSessionMaxLeaseInvalidRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	writeYAML(t, tmp, "session:\n  max_lease: \"notaduration\"\n")

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := Resolve(nil, dir)
	if err == nil {
		t.Fatal("expected error for invalid max_lease, got nil")
	}
	if !strings.Contains(err.Error(), "session.max_lease") {
		t.Errorf("error %q does not mention session.max_lease", err.Error())
	}
}

func TestResolveSessionMaxLeaseDefaultIsOff(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HUM_HOME", tmp)

	dir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	c, prov, err := Resolve(nil, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.Session.MaxLease != "" {
		t.Errorf("MaxLease = %q, want empty (off by default)", c.Session.MaxLease)
	}
	if prov["session.max_lease"] != LayerDefault {
		t.Errorf("prov[session.max_lease] = %q, want default", prov["session.max_lease"])
	}
}
