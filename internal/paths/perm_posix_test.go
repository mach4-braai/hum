//go:build !windows

package paths

import (
	"os"
	"testing"
)

func assertPrivateDir(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %q: %v", dir, err)
	}
	if perm := info.Mode().Perm(); perm != RuntimeDirPerm {
		t.Errorf("permissions on %q = %#o, want %#o", dir, perm, RuntimeDirPerm)
	}
}
