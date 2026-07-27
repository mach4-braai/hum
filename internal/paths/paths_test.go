package paths

import "testing"

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
