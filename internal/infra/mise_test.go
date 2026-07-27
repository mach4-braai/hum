//go:build infra

// These assertions shell out to the mise binary, so they are kept behind a build
// tag: a plain `go test ./...` must stay runnable with nothing but a Go
// toolchain. The `check` task runs this suite explicitly, so CI still validates
// the build contract.
package infra

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requiredTasks is the build contract that CI, the release pipeline and
// contributors all rely on. A missing task surfaces as a confusing CI failure
// rather than an obvious one, so their existence is asserted rather than assumed.
var requiredTasks = []string{"build", "check", "clean", "fmt", "install", "test", "vet"}

// Listing goes through mise itself rather than a TOML parser: it proves mise can
// actually read the file, and it keeps the dependency budget in PRD.md
// section 22 at the two libraries the daemon genuinely needs.
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

// The binaries must report a real version under Homebrew, so the canonical build
// has to stamp them. GoReleaser and the Homebrew formula cannot use mise and so
// duplicate these flags; this assertion is what makes that duplication
// detectable once those files exist.
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

// The toolchain is pinned in mise.toml so contributors, CI and release builds
// use the same compiler. go.mod declares the language version, which is a
// different thing and may legitimately differ.
func TestMisePinsGoToolchain(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	if !strings.Contains(string(data), "go =") {
		t.Error("mise.toml does not pin a go version under [tools]")
	}
}
