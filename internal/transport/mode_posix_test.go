//go:build !windows

package transport_test

import (
	"os"
	"testing"
)

func assertSocketMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode: want 0600, got %04o", got)
	}
}
