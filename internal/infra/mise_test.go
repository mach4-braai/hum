//go:build infra

// These assertions shell out to mise, so they sit behind a build tag: a plain
// `go test ./...` must need nothing but a Go toolchain. `check` runs them.
package infra

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requiredTasks is the build contract CI and the release pipeline depend on.
var requiredTasks = []string{"build", "check", "clean", "fmt", "install", "test", "vet"}

// Listing goes through mise itself: it proves mise can read the file, and avoids
// spending a dependency on a TOML parser.
func TestMiseDefinesRequiredTasks(t *testing.T) {
	cmd := exec.Command("mise", "tasks", "ls", "-J")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mise tasks ls: %v", err)
	}

	var tasks []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &tasks); err != nil {
		t.Fatalf("parse mise task list: %v\n%s", err, out)
	}
	have := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		have[task.Name] = true
	}

	for _, want := range requiredTasks {
		if !have[want] {
			t.Errorf("mise task %q is not defined", want)
		}
	}
}

// GoReleaser and the Homebrew formula cannot use mise, so they duplicate these
// flags; this assertion is what makes the duplication detectable when it drifts.
func TestMiseBuildTaskStampsVersionAndTrimsPaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	config := string(data)

	for _, want := range []string{"-trimpath", "-X main.version=", "-X main.commit=", "-X main.date="} {
		if !strings.Contains(config, want) {
			t.Errorf("mise.toml does not set %q for the build task", want)
		}
	}
}

// The toolchain is pinned here so contributors, CI and releases share a compiler.
func TestMisePinsGoToolchain(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	if !strings.Contains(string(data), "go =") {
		t.Error("mise.toml does not pin a go version under [tools]")
	}
}
