package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found in any parent directory")
		}
		dir = parent
	}
}

func TestGitignoreExcludesBuildArtefacts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	lines := make(map[string]bool)
	for _, l := range strings.Split(string(data), "\n") {
		lines[strings.TrimSpace(l)] = true
	}

	for _, want := range []string{"/bin/", "/dist/", "/hum", "/humd", "*.test", "coverage.out"} {
		if !lines[want] {
			t.Errorf(".gitignore is missing the entry %q", want)
		}
	}
	for _, unanchored := range []string{"hum", "humd"} {
		if lines[unanchored] {
			t.Errorf(".gitignore has unanchored %q, which would also ignore the cmd/%s source directory", unanchored, unanchored)
		}
	}
}
