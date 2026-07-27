package infra

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var requiredTasks = []string{"build", "check", "clean", "coverage", "fmt", "install", "test", "vet"}

// Via mise itself, proving mise can read the file. Skip only when mise is absent;
// other failures may mean a malformed mise.toml.
func TestMiseDefinesRequiredTasks(t *testing.T) {
	if _, err := exec.LookPath("mise"); err != nil {
		t.Skip("mise is not installed; skipping the task-list assertion")
	}
	root := repoRoot(t)
	cmd := exec.Command("mise", "tasks", "ls", "-J")
	cmd.Dir = root
	// Trust for this invocation only, so trust state cannot affect the result.
	cmd.Env = append(os.Environ(), "MISE_TRUSTED_CONFIG_PATHS="+root)
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

// GoReleaser and the formula duplicate these flags; this detects the drift.
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

func TestMisePinsGoToolchain(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	if !strings.Contains(string(data), "go =") {
		t.Error("mise.toml does not pin a go version under [tools]")
	}
}

// The threshold is a ratchet; without enforcement the coverage job is decorative.
func TestMiseCoverageTaskEnforcesAMinimum(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	_, rest, found := strings.Cut(string(data), "[tasks.coverage]")
	if !found {
		t.Fatal("mise.toml defines no coverage task")
	}
	task, _, _ := strings.Cut(rest, "\n[tasks.")

	for _, want := range []string{"-coverprofile=coverage.out", "MINIMUM="} {
		if !strings.Contains(task, want) {
			t.Errorf("the coverage task does not contain %q", want)
		}
	}
}
