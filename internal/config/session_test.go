package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mach4-braai/hum/internal/paths"
)

func writeProjectConfig(t *testing.T, dir, body string) {
	t.Helper()
	hum := filepath.Join(dir, paths.ProjectDirName)
	if err := os.MkdirAll(hum, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hum, paths.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveForSessionReadsTheProjectRoot(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	project := t.TempDir()
	writeProjectConfig(t, project, "music:\n  root: A\n  scale: dorian\n")

	cfg, prov, err := ResolveForSession("", project)
	if err != nil {
		t.Fatalf("ResolveForSession(%q): %v", project, err)
	}
	if cfg.Music.Root != "A" || cfg.Music.Scale != "dorian" {
		t.Errorf("resolved music = %+v, want root A scale dorian", cfg.Music)
	}
	if prov["music.root"] != LayerProject {
		t.Errorf("provenance for music.root = %q, want %q", prov["music.root"], LayerProject)
	}
}

func TestResolveForSessionIgnoresTheWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	cwd := t.TempDir()
	writeProjectConfig(t, cwd, "music:\n  root: F\n")
	t.Chdir(cwd)

	cfg, prov, err := ResolveForSession("", "")
	if err != nil {
		t.Fatalf("ResolveForSession(\"\"): %v", err)
	}
	if cfg.Music.Root != Default().Music.Root {
		t.Errorf("music.root = %q, want the default %q; a session with no root must not inherit the daemon's directory", cfg.Music.Root, Default().Music.Root)
	}
	if prov["music.root"] != LayerDefault {
		t.Errorf("provenance for music.root = %q, want %q", prov["music.root"], LayerDefault)
	}
}

func TestResolveForSessionHonoursGlobalWithoutARoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(filepath.Join(home, paths.ConfigFileName), []byte("music:\n  scale: lydian\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, prov, err := ResolveForSession("", "")
	if err != nil {
		t.Fatalf("ResolveForSession(\"\"): %v", err)
	}
	if cfg.Music.Scale != "lydian" {
		t.Errorf("music.scale = %q, want lydian from global config", cfg.Music.Scale)
	}
	if prov["music.scale"] != LayerGlobal {
		t.Errorf("provenance for music.scale = %q, want %q", prov["music.scale"], LayerGlobal)
	}
}

func TestResolveForSessionRejectsABadRoot(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"relative":    "projects/tofu",
		"nonexistent": filepath.Join(t.TempDir(), "absent"),
		"not a dir":   file,
	}
	for name, root := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ResolveForSession("", root); !errors.Is(err, ErrProjectRoot) {
				t.Errorf("ResolveForSession(%q) error = %v, want ErrProjectRoot", root, err)
			}
		})
	}
}

func TestResolveForSessionPrefersProjectOverGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(filepath.Join(home, paths.ConfigFileName), []byte("music:\n  root: G\n  theme: minimal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	writeProjectConfig(t, project, "music:\n  root: A\n")

	cfg, prov, err := ResolveForSession("", project)
	if err != nil {
		t.Fatalf("ResolveForSession(%q): %v", project, err)
	}
	if cfg.Music.Root != "A" {
		t.Errorf("music.root = %q, want A from the project layer", cfg.Music.Root)
	}
	if prov["music.theme"] != LayerGlobal {
		t.Errorf("provenance for music.theme = %q, want %q; an untouched field falls through", prov["music.theme"], LayerGlobal)
	}
}

func TestResolveSourcesHonoursAnExplicitGlobalFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(filepath.Join(home, paths.ConfigFileName), []byte("music:\n  scale: lydian\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	elsewhere := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(elsewhere, []byte("music:\n  scale: phrygian\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, prov, err := ResolveForSession(elsewhere, "")
	if err != nil {
		t.Fatalf("ResolveForSession(%q, \"\"): %v", elsewhere, err)
	}
	if cfg.Music.Scale != "phrygian" {
		t.Errorf("music.scale = %q, want phrygian from the explicit global file", cfg.Music.Scale)
	}
	if prov["music.scale"] != LayerGlobal {
		t.Errorf("provenance for music.scale = %q, want %q", prov["music.scale"], LayerGlobal)
	}
}
