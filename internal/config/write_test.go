package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPatchPreservesUnrelatedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "project:\n  name: tofu\nmusic:\n  root: A\n  scale: dorian\naudio:\n  volume: 0.2\n")

	if err := Patch(path, map[string]string{"audio.volume": "0.4", "audio.muted": "true"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Project.Name != "tofu" {
		t.Errorf("project.name = %q, want tofu preserved through the rewrite", got.Project.Name)
	}
	if got.Music.Root != "A" || got.Music.Scale != "dorian" {
		t.Errorf("music = %+v, want A/dorian preserved", got.Music)
	}
	if got.Audio.Volume != 0.4 {
		t.Errorf("audio.volume = %v, want 0.4", got.Audio.Volume)
	}
	if !got.Audio.Muted {
		t.Error("audio.muted = false, want the patched value")
	}
}

func TestPatchPreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "# valid scales: dorian, major\nmusic:\n  scale: dorian # the current choice\n")

	if err := Patch(path, map[string]string{"music.theme": "minimal"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"# valid scales: dorian, major", "# the current choice"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("rewritten config = %q, want it to keep %q", data, want)
		}
	}
}

func TestPatchCreatesTheFileAndItsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	if err := Patch(path, map[string]string{"music.theme": "minimal"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Music.Theme != "minimal" {
		t.Errorf("music.theme = %q, want minimal", got.Music.Theme)
	}
	assertFilePerm(t, path, configFilePerm)
}

func TestPatchReplacesAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "")

	if err := Patch(path, map[string]string{"audio.muted": "true"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Audio.Muted {
		t.Error("audio.muted = false, want the patched value in a file that had no mapping")
	}
}

func TestPatchReplacesAScalarWhereAMappingIsNeeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "audio: loud\n")

	if err := Patch(path, map[string]string{"audio.volume": "0.5"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Audio.Volume != 0.5 {
		t.Errorf("audio.volume = %v, want 0.5", got.Audio.Volume)
	}
}

func TestPatchRejectsBadValuesWithoutTouchingTheFile(t *testing.T) {
	original := "audio:\n  volume: 0.2\n"

	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"volume above range", map[string]string{"audio.volume": "1.5"}, "out of range"},
		{"volume not a number", map[string]string{"audio.volume": "loud"}, "not a valid number"},
		{"volume NaN", map[string]string{"audio.volume": "NaN"}, "out of range"},
		{"muted not a boolean", map[string]string{"audio.muted": "maybe"}, "not a valid boolean"},
		{"empty theme", map[string]string{"music.theme": ""}, "must not be empty"},
		{"unknown key", map[string]string{"music.tempo": "120"}, "unknown config key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, path, original)

			err := Patch(path, tc.values)

			if err == nil {
				t.Fatalf("Patch(%v) = nil, want an error", tc.values)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read: %v", readErr)
			}
			if string(data) != original {
				t.Errorf("config = %q, want it untouched after a rejected patch", data)
			}
		})
	}
}

func TestPatchUnknownKeyIsDistinguishable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	err := Patch(path, map[string]string{"music.tempo": "120"})

	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("error = %v, want it to wrap ErrUnknownKey", err)
	}
}

func TestPatchRejectsAFileThatIsNotAMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "- one\n- two\n")

	err := Patch(path, map[string]string{"audio.muted": "true"})

	if err == nil || !strings.Contains(err.Error(), "not a YAML mapping") {
		t.Errorf("error = %v, want it to reject a non-mapping document", err)
	}
}

func TestPatchReportsUnreadableAndUnparseableFiles(t *testing.T) {
	dir := t.TempDir()

	broken := filepath.Join(dir, "broken.yaml")
	writeFile(t, broken, "music:\n\tscale: dorian\n")
	if err := Patch(broken, map[string]string{"audio.muted": "true"}); err == nil {
		t.Error("Patch on unparseable YAML = nil, want an error")
	}

	if err := Patch(dir, map[string]string{"audio.muted": "true"}); err == nil {
		t.Error("Patch on a directory = nil, want an error")
	}
}

func TestWriteFailsWhenTheDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a directory\n")

	err := Write(filepath.Join(blocker, "config.yaml"), []byte("audio: {}\n"))

	if err == nil {
		t.Fatal("Write below a regular file = nil, want an error")
	}
}

func TestWriteReportsATemporaryFileItCannotCreate(t *testing.T) {
	original := osCreateTemp
	t.Cleanup(func() { osCreateTemp = original })
	osCreateTemp = func(string, string) (*os.File, error) { return nil, os.ErrPermission }

	err := Write(filepath.Join(t.TempDir(), "config.yaml"), []byte("audio: {}\n"))

	if err == nil || !strings.Contains(err.Error(), "temporary file") {
		t.Errorf("error = %v, want it to name the temporary file it could not create", err)
	}
}

func TestWriteLeavesTheTargetAloneWhenTheTemporaryFileIsUnwritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "audio:\n  volume: 0.2\n")

	original := osCreateTemp
	t.Cleanup(func() { osCreateTemp = original })
	osCreateTemp = func(dir, pattern string) (*os.File, error) {
		f, err := original(dir, pattern)
		if err != nil {
			return nil, err
		}
		return f, f.Close()
	}

	if err := Write(path, []byte("audio:\n  volume: 0.9\n")); err == nil {
		t.Fatal("Write to a closed temporary file = nil, want an error")
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Audio.Volume != 0.2 {
		t.Errorf("audio.volume = %v, want the original 0.2 left intact", got.Audio.Volume)
	}
}

type failingEncoder struct {
	encodeErr error
	closeErr  error
}

func (f *failingEncoder) SetIndent(int)    {}
func (f *failingEncoder) Encode(any) error { return f.encodeErr }
func (f *failingEncoder) Close() error     { return f.closeErr }

func TestPatchReportsThePathWhenEncodingFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "audio:\n  volume: 0.2\n")

	t.Cleanup(func() { newDocumentEncoder = newYAMLEncoder })
	boom := errors.New("encoder boom")
	newDocumentEncoder = func(io.Writer) documentEncoder { return &failingEncoder{encodeErr: boom} }

	err := Patch(path, map[string]string{"audio.volume": "0.4"})
	if err == nil {
		t.Fatal("Patch = nil, want the encode failure reported")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the encoder error", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the path named so the user knows which file failed", err)
	}
}

func TestEncodeFailsWhenTheEncoderCannotClose(t *testing.T) {
	t.Cleanup(func() { newDocumentEncoder = newYAMLEncoder })
	boom := errors.New("close boom")
	newDocumentEncoder = func(io.Writer) documentEncoder { return &failingEncoder{closeErr: boom} }

	node := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
		{Kind: yaml.MappingNode, Tag: "!!map"},
	}}
	if _, err := encode(node); !errors.Is(err, boom) {
		t.Errorf("encode = %v, want it to wrap the close error: a document that cannot be flushed is not a document", err)
	}
}

func TestPatchSessionMaxLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "music:\n  root: C\n  scale: major\n  theme: minimal\n")

	if err := Patch(path, map[string]string{"session.max_lease": "24h"}); err != nil {
		t.Fatalf("Patch session.max_lease: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Session.MaxLease != "24h" {
		t.Errorf("session.max_lease = %q, want 24h", got.Session.MaxLease)
	}

	if err := Patch(path, map[string]string{"session.max_lease": ""}); err != nil {
		t.Fatalf("Patch session.max_lease to empty: %v", err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatalf("Load after clearing: %v", err)
	}
	if got.Session.MaxLease != "" {
		t.Errorf("session.max_lease = %q, want empty after clear", got.Session.MaxLease)
	}
}

func TestPatchSessionMaxLeaseRejectsBadValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "music:\n  root: C\n  scale: major\n  theme: minimal\n")

	for _, bad := range []string{"forever", "-1h"} {
		err := Patch(path, map[string]string{"session.max_lease": bad})
		if err == nil {
			t.Errorf("Patch session.max_lease=%q = nil, want error", bad)
		}
	}
}
