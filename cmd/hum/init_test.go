package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/theme"
)

func initChdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestInitWritesValidConfigWithGitRoot(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}

	configPath := filepath.Join(dir, paths.ProjectDirName, paths.ConfigFileName)
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("config not found after init")
	}
	if cfg.Project.Name != filepath.Base(dir) {
		t.Errorf("project.name = %q, want %q", cfg.Project.Name, filepath.Base(dir))
	}
	d := config.Default()
	if cfg.Music.Root != d.Music.Root {
		t.Errorf("music.root = %q, want %q", cfg.Music.Root, d.Music.Root)
	}
	if cfg.Music.Scale != d.Music.Scale {
		t.Errorf("music.scale = %q, want %q", cfg.Music.Scale, d.Music.Scale)
	}
	if cfg.Music.Theme != d.Music.Theme {
		t.Errorf("music.theme = %q, want %q", cfg.Music.Theme, d.Music.Theme)
	}
	if cfg.Audio.Volume != d.Audio.Volume {
		t.Errorf("audio.volume = %v, want %v", cfg.Audio.Volume, d.Audio.Volume)
	}
	if cfg.Audio.Muted != d.Audio.Muted {
		t.Errorf("audio.muted = %v, want %v", cfg.Audio.Muted, d.Audio.Muted)
	}
}

func TestInitProjectNameWithoutGit(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}

	configPath := filepath.Join(dir, paths.ProjectDirName, paths.ConfigFileName)
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Project.Name != filepath.Base(dir) {
		t.Errorf("project.name = %q, want %q", cfg.Project.Name, filepath.Base(dir))
	}
}

func TestInitCommentsListAllScalesAndThemes(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"init"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}

	configPath := filepath.Join(dir, paths.ProjectDirName, paths.ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	for _, scale := range harmony.ScaleNames() {
		if !strings.Contains(content, scale) {
			t.Errorf("scale %q not found in generated config", scale)
		}
	}
	for _, th := range theme.List() {
		if !strings.Contains(content, th) {
			t.Errorf("theme %q not found in generated config", th)
		}
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"init"}, &stdout1, &stderr1); code != exitOK {
		t.Fatalf("first init exit %d", code)
	}

	configPath := filepath.Join(dir, paths.ProjectDirName, paths.ConfigFileName)
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var stdout2, stderr2 bytes.Buffer
	code := run([]string{"init"}, &stdout2, &stderr2)
	if code != exitDaemonError {
		t.Fatalf("second init exit %d, want %d", code, exitDaemonError)
	}
	if !strings.Contains(stderr2.String(), configPath) {
		t.Errorf("stderr %q does not contain path %q", stderr2.String(), configPath)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Error("file was modified without --force")
	}

	var stdout3, stderr3 bytes.Buffer
	if code := run([]string{"init", "--force"}, &stdout3, &stderr3); code != exitOK {
		t.Fatalf("init --force exit %d; stderr=%q", code, stderr3.String())
	}
}

func TestInitGlobalWritesGlobalConfig(t *testing.T) {
	humHome := t.TempDir()
	t.Setenv("HUM_HOME", humHome)
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init", "--global"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}

	globalPath := paths.GlobalConfigFile()
	if _, err := os.Stat(globalPath); err != nil {
		t.Errorf("global config not created: %v", err)
	}

	localDir := filepath.Join(dir, paths.ProjectDirName)
	if _, err := os.Stat(localDir); err == nil {
		t.Error(".hum directory created in working directory when --global was used")
	}
}

func TestInitPrintOutputIsLoadable(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init", "--print"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	if out == "" {
		t.Fatal("--print produced no output")
	}

	tmpFile := filepath.Join(t.TempDir(), "test.yaml")
	if err := os.WriteFile(tmpFile, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(tmpFile)
	if err != nil {
		t.Errorf("config.Load on --print output: %v", err)
	}
	if cfg == nil {
		t.Error("config.Load returned nil for --print output")
	}

	localDir := filepath.Join(dir, paths.ProjectDirName)
	if _, err := os.Stat(localDir); err == nil {
		t.Error(".hum directory created when --print was used")
	}
}

func TestInitPrintWithForceDoesNotError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"init"}, &stdout1, &stderr1); code != exitOK {
		t.Fatalf("first init exit %d", code)
	}

	var stdout2, stderr2 bytes.Buffer
	code := run([]string{"init", "--print", "--force"}, &stdout2, &stderr2)
	if code != exitOK {
		t.Fatalf("--print --force exit %d; stderr=%q", code, stderr2.String())
	}
}

func TestInitWriteFailureReportsPath(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	humPath := filepath.Join(dir, paths.ProjectDirName)
	if err := os.WriteFile(humPath, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"init"}, &stdout, &stderr)
	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d", code, exitDaemonError)
	}

	expected := filepath.Join(dir, paths.ProjectDirName, paths.ConfigFileName)
	if !strings.Contains(stderr.String(), expected) {
		t.Errorf("stderr %q does not contain path %q", stderr.String(), expected)
	}
}

func TestInitStrayPositionalIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init", "extra"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}

func TestInitPrintDoesNotCheckExisting(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	dir := t.TempDir()
	initChdir(t, dir)

	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"init"}, &stdout1, &stderr1); code != exitOK {
		t.Fatalf("first init exit %d", code)
	}

	var stdout2, stderr2 bytes.Buffer
	code := run([]string{"init", "--print"}, &stdout2, &stderr2)
	if code != exitOK {
		t.Fatalf("--print after existing file exit %d; stderr=%q", code, stderr2.String())
	}
	if stdout2.String() == "" {
		t.Error("--print produced no output")
	}
}
func TestInitUnparsableFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	code := run([]string{"init", "--bogus"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want a usage error message")
	}
}

func TestInitProjectNameFallsBackToFinalPathElement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gone", "widget")
	name := initProjectName(dir)
	if name != "widget" {
		t.Errorf("initProjectName = %q, want %q", name, "widget")
	}
}
