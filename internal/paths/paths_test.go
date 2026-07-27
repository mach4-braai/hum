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
