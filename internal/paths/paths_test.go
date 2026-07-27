package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// HUM_HOME is the documented escape hatch for relocating Hum's state. Tests and
// sandboxed installs depend on it, so it takes precedence over the home
// directory rather than merely seeding a default.
func TestGlobalConfigDirPrefersHumHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUM_HOME", dir)

	if got := GlobalConfigDir(); got != dir {
		t.Errorf("GlobalConfigDir() = %q, want %q", got, dir)
	}
}

// With no override, configuration lives at ~/.hum so that Hum behaves like a
// conventional Unix tool and needs no setup on a fresh machine.
func TestGlobalConfigDirFallsBackToHomeDotHum(t *testing.T) {
	t.Setenv("HUM_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	want := filepath.Join(home, ".hum")

	if got := GlobalConfigDir(); got != want {
		t.Errorf("GlobalConfigDir() = %q, want %q", got, want)
	}
}

// The filename is part of the documented layout in PRD.md section 12, so it is
// asserted rather than left to the caller to join.
func TestGlobalConfigFileLivesInsideConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUM_HOME", dir)
	want := filepath.Join(dir, "config.yaml")

	if got := GlobalConfigFile(); got != want {
		t.Errorf("GlobalConfigFile() = %q, want %q", got, want)
	}
}

// Clients are invoked from wherever the developer happens to be inside a
// project, so discovery walks upward like git does. Requiring the exact project
// root would make project config unusable in practice.
func TestProjectConfigFileFoundByWalkingUpFromNestedDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".hum"), 0o755); err != nil {
		t.Fatalf("mkdir .hum: %v", err)
	}
	want := filepath.Join(root, ".hum", "config.yaml")
	if err := os.WriteFile(want, []byte("project:\n  name: demo\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	nested := filepath.Join(root, "cmd", "deep", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	got, ok := ProjectConfigFile(nested)

	if !ok {
		t.Fatalf("ProjectConfigFile(%q) reported not found, want %q", nested, want)
	}
	if got != want {
		t.Errorf("ProjectConfigFile(%q) = %q, want %q", nested, got, want)
	}
}
