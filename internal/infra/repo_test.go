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

	for _, want := range []string{"/.gocache/", "/bin/", "/dist/", "/hum", "/humd", "*.test", "coverage.out"} {
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

func TestDependabotWatchesEveryEcosystemThisRepoHas(t *testing.T) {
	config := readRepoFile(t, ".github", "dependabot.yaml")

	for _, want := range []string{"package-ecosystem: github-actions", "package-ecosystem: gomod"} {
		if !strings.Contains(config, want) {
			t.Errorf("dependabot.yaml has no %q, so those dependencies are never offered an update", want)
		}
	}
	var actions []string
	for _, pattern := range []string{"action.yml", "action.yaml"} {
		matches, err := filepath.Glob(filepath.Join(repoRoot(t), ".github", "actions", "*", pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		actions = append(actions, matches...)
	}
	for _, path := range actions {
		dir := "/.github/actions/" + filepath.Base(filepath.Dir(path))
		if !strings.Contains(config, dir) {
			t.Errorf("dependabot.yaml does not list %s, so that action's own pins would go stale", dir)
		}
	}
	for _, prefix := range []string{"prefix: ci", "prefix: chore"} {
		if !strings.Contains(config, prefix) {
			t.Errorf("dependabot.yaml does not set %q, so bumps appear in the release notes .goreleaser.yaml filters by that prefix", prefix)
		}
	}
}

func TestCodeownersOwnsEverythingButTheDocs(t *testing.T) {
	owners := readRepoFile(t, ".github", "CODEOWNERS")

	if !strings.Contains(owners, "* @mcgeerdev") {
		t.Fatal("CODEOWNERS claims no default owner, so a pull request touching the protocol or the release pipeline requests no review")
	}
	docs := false
	for _, line := range strings.Split(owners, "\n") {
		if strings.TrimSpace(line) == "/docs/" {
			docs = true
		}
	}
	if !docs {
		t.Error("CODEOWNERS does not leave /docs/ unowned, so documentation changes drag in a review the owner did not ask for")
	}
}
