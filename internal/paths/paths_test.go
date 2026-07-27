package paths

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalConfigDirPrefersHumHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUM_HOME", dir)

	if got := GlobalConfigDir(); got != dir {
		t.Errorf("GlobalConfigDir() = %q, want %q", got, dir)
	}
}

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

func TestGlobalConfigFileLivesInsideConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HUM_HOME", dir)
	want := filepath.Join(dir, "config.yaml")

	if got := GlobalConfigFile(); got != want {
		t.Errorf("GlobalConfigFile() = %q, want %q", got, want)
	}
}

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

func TestProjectConfigFileReportsNotFound(t *testing.T) {
	t.Run("no project config anywhere above", func(t *testing.T) {
		if got, ok := ProjectConfigFile(t.TempDir()); ok {
			t.Errorf("ProjectConfigFile() = %q, true; want not found", got)
		}
	})

	t.Run("global config is not project config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("HUM_HOME", "")
		if err := os.MkdirAll(filepath.Join(home, ".hum"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(home, ".hum", "config.yaml"), []byte("music:\n  root: D\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		nested := filepath.Join(home, "projects", "thing")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if got, ok := ProjectConfigFile(nested); ok {
			t.Errorf("ProjectConfigFile() = %q, true; want not found, because that path is the global config file", got)
		}
	})
}

func TestProjectConfigFileAcceptsRelativeStartDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HUM_HOME", filepath.Join(root, "unrelated-home"))
	if err := os.MkdirAll(filepath.Join(root, ".hum"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := filepath.Join(root, ".hum", "config.yaml")
	if err := os.WriteFile(want, []byte("project:\n  name: demo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	nested := filepath.Join(root, "cmd", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(root)

	got, ok := ProjectConfigFile(filepath.Join("cmd", "deep"))

	if !ok {
		t.Fatalf(`ProjectConfigFile("cmd/deep") reported not found, want %q`, want)
	}
	if got != want {
		t.Errorf(`ProjectConfigFile("cmd/deep") = %q, want %q`, got, want)
	}
}

func TestSocketPathResolution(t *testing.T) {
	t.Run("prefers HUM_SOCKET", func(t *testing.T) {
		want := filepath.Join(t.TempDir(), "custom.sock")
		t.Setenv("HUM_SOCKET", want)
		t.Setenv("HUM_HOME", t.TempDir())

		if got := SocketPath(); got != want {
			t.Errorf("SocketPath() = %q, want %q", got, want)
		}
	})

	t.Run("defaults inside the config dir", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HUM_SOCKET", "")
		t.Setenv("HUM_HOME", home)
		want := filepath.Join(home, "humd.sock")

		if got := SocketPath(); got != want {
			t.Errorf("SocketPath() = %q, want %q", got, want)
		}
	})
}

const maxSocketPathLen = 100

func TestDefaultSocketPathStaysWithinSunPathBudget(t *testing.T) {
	t.Setenv("HUM_SOCKET", "")
	t.Setenv("HUM_HOME", "/Users/somebodywithalongishname/.hum")

	if got := SocketPath(); len(got) > maxSocketPathLen {
		t.Errorf("SocketPath() = %q is %d bytes, want at most %d", got, len(got), maxSocketPathLen)
	}
}

func TestEnsureRuntimeDirCreatesSocketParentPrivately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_SOCKET", "")
	t.Setenv("HUM_HOME", filepath.Join(home, "state", "nested"))

	if err := EnsureRuntimeDir(); err != nil {
		t.Fatalf("EnsureRuntimeDir() = %v, want nil", err)
	}

	dir := filepath.Dir(SocketPath())
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %q: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("permissions on %q = %#o, want %#o", dir, perm, 0o700)
	}
}

func TestEnsureRuntimeDirIsIdempotent(t *testing.T) {
	t.Setenv("HUM_SOCKET", "")
	t.Setenv("HUM_HOME", t.TempDir())

	if err := EnsureRuntimeDir(); err != nil {
		t.Fatalf("first EnsureRuntimeDir() = %v, want nil", err)
	}
	if err := EnsureRuntimeDir(); err != nil {
		t.Errorf("second EnsureRuntimeDir() = %v, want nil", err)
	}
}

func TestGlobalConfigDirFallsBackToProjectDirNameWithoutAHome(t *testing.T) {
	t.Setenv("HUM_HOME", "")
	t.Setenv("HOME", "")

	if got := GlobalConfigDir(); got != ProjectDirName {
		t.Errorf("GlobalConfigDir() = %q, want %q: an absent home must not yield a bare or rooted path", got, ProjectDirName)
	}
}

func withRemovedWorkingDir(t *testing.T) {
	t.Helper()
	original := absolute
	t.Cleanup(func() { absolute = original })
	absolute = func(p string) (string, error) {
		if filepath.IsAbs(p) {
			return filepath.Clean(p), nil
		}
		return "", errors.New("getwd: no such file or directory")
	}
}

func TestProjectConfigFileReportsNotFoundWhenTheWorkingDirIsGone(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	withRemovedWorkingDir(t)

	got, ok := ProjectConfigFile("cmd")

	if ok {
		t.Errorf("ProjectConfigFile(%q) = %q, true; want not found when the start dir cannot be resolved", "cmd", got)
	}
	if got != "" {
		t.Errorf("ProjectConfigFile(%q) = %q, want an empty path alongside false", "cmd", got)
	}
}

func TestProjectConfigFileWalksWhenTheGlobalPathCannotBeMadeAbsolute(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ProjectDirName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := filepath.Join(root, ProjectDirName, ConfigFileName)
	if err := os.WriteFile(want, []byte("project:\n  name: demo\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("HUM_HOME", "relative-home")
	withRemovedWorkingDir(t)

	got, ok := ProjectConfigFile(root)

	if !ok {
		t.Fatalf("ProjectConfigFile(%q) reported not found, want %q", root, want)
	}
	if got != want {
		t.Errorf("ProjectConfigFile(%q) = %q, want %q", root, got, want)
	}
}

func TestEnsureRuntimeDirReportsAnUncreatableParent(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("HUM_SOCKET", filepath.Join(blocker, "nested", "humd.sock"))

	err := EnsureRuntimeDir()

	if err == nil {
		t.Fatal("EnsureRuntimeDir() = nil, want an error when the parent is a regular file")
	}
	if !strings.Contains(err.Error(), blocker) {
		t.Errorf("EnsureRuntimeDir() = %v, want the offending path %q named", err, blocker)
	}
}
