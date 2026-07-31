//go:build !windows

package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertFilePerm(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != want {
		t.Errorf("mode = %o, want %o", perm, want)
	}
}

func TestPatchPreservesTheFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "audio:\n  volume: 0.2\n")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	if err := Patch(path, map[string]string{"audio.volume": "0.4"}); err == nil {
		t.Fatal("Patch into a read-only directory = nil, want an error")
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Audio.Volume != 0.2 {
		t.Errorf("audio.volume = %v, want the original 0.2 left intact", got.Audio.Volume)
	}
}

func TestWriteReportsRenameFailure(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	original := osCreateTemp
	t.Cleanup(func() { osCreateTemp = original })
	osCreateTemp = func(d, pattern string) (*os.File, error) {
		f, err := original(d, pattern)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(f.Name()); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}

	path := filepath.Join(dir, "config.yaml")
	err = Write(path, []byte("audio: {}\n"))

	if err == nil {
		t.Fatal("Write with unlinked temp file = nil, want an error")
	}
	if !strings.Contains(err.Error(), "replace") || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want it to mention replace and %s", err, path)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination file exists or stat returned unexpected error: %v", statErr)
	}
}

func TestWriteReportsSyncFailureOnPipe(t *testing.T) {
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	var r, w *os.File
	original := osCreateTemp
	t.Cleanup(func() { osCreateTemp = original })
	t.Cleanup(func() {
		if r != nil {
			r.Close()
		}
		if w != nil {
			w.Close()
		}
	})
	osCreateTemp = func(_, _ string) (*os.File, error) {
		var pipeErr error
		r, w, pipeErr = os.Pipe()
		if pipeErr != nil {
			return nil, pipeErr
		}
		return w, nil
	}

	path := filepath.Join(dir, "config.yaml")
	err = Write(path, []byte("audio: {}\n"))

	if err == nil {
		t.Fatal("Write using a pipe as temp file = nil, want an error")
	}
	if !strings.Contains(err.Error(), "write ") {
		t.Errorf("error = %v, want it to be wrapped as \"write <name>: ...\"", err)
	}
}
