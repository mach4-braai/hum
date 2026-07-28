package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPRDFixture(t *testing.T) {
	c, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c == nil {
		t.Fatal("Load: got nil config")
	}
	if c.Music.Root != "D" {
		t.Errorf("Root = %q, want D", c.Music.Root)
	}
	if c.Music.Scale != "dorian" {
		t.Errorf("Scale = %q, want dorian", c.Music.Scale)
	}
	if c.Music.Theme != "orchestra" {
		t.Errorf("Theme = %q, want orchestra", c.Music.Theme)
	}
	if c.Project.Name != "tofu" {
		t.Errorf("Name = %q, want tofu", c.Project.Name)
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatal("expected nil config, got non-nil")
	}
}

func TestLoadUnknownField(t *testing.T) {
	_, err := Load("testdata/unknown_field.yaml")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "testdata/unknown_field.yaml") {
		t.Errorf("error %q missing file path", msg)
	}
	if !strings.Contains(msg, "theme") {
		t.Errorf("error %q missing field name", msg)
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	_, err := Load("testdata/malformed.yaml")
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "testdata/malformed.yaml") {
		t.Errorf("error %q missing file path", err.Error())
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	orig := osOpen
	t.Cleanup(func() { osOpen = orig })
	osOpen = func(name string) (*os.File, error) {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
	}

	_, err := Load("some.yaml")
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "some.yaml") {
		t.Errorf("error %q missing file path", err.Error())
	}
}
