package config

import (
	"io/fs"
	"os"
	"testing"
)

func assertFilePerm(t *testing.T, path string, _ fs.FileMode) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}
