package infra

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var requiredTasks = []string{"build", "check", "clean", "coverage", "e2e", "fmt", "fuzz", "install", "junit", "mutate", "snapshot", "test", "vet", "vuln"}

func TestMiseDefinesRequiredTasks(t *testing.T) {
	if _, err := exec.LookPath("mise"); err != nil {
		t.Skip("mise is not installed; skipping the task-list assertion")
	}
	root := repoRoot(t)
	cmd := exec.Command("mise", "tasks", "ls", "-J")
	cmd.Dir = root
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

func TestMiseCoverageTaskFailsOnABlockThatNeverRuns(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	_, rest, found := strings.Cut(string(data), "[tasks.coverage]")
	if !found {
		t.Fatal("mise.toml defines no coverage task")
	}
	task, _, _ := strings.Cut(rest, "\n[tasks.")

	if !strings.Contains(task, "never executed:") {
		t.Error("the coverage task does not scan coverage.out for blocks that never run; go tool cover rounds a single uncovered statement away and the percentage alone would pass")
	}
	if !strings.Contains(task, "hits[$1] += $3") {
		t.Error("the coverage task does not sum hit counts per block; a block appears once per test binary and judging one section alone reports covered code as dead")
	}
	if !strings.Contains(task, "UNEXERCISABLE=") {
		t.Error("the coverage task carries no exemption for the audio device block, so it cannot pass at all")
	}
}

func TestMiseCoverageTaskEmitsTheSummaryCIPublishes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	if !strings.Contains(string(data), "of statements, minimum") {
		t.Fatal("the coverage task emits no summary line for CI to publish")
	}

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	if !strings.Contains(string(workflow), `*"of statements, minimum"*`) {
		t.Error("the workflow does not check the summary shape, so a malformed line would be published as the description")
	}
}

func TestMisePinsTheOneMutationTesterThatRuns(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	if !strings.Contains(string(data), `"go:github.com/avito-tech/go-mutesting/cmd/go-mutesting"`) {
		t.Error("mise.toml pins no mutation tester, so the mutate task depends on whatever the machine happens to have; gremlins times out on 114 of 116 mutants here and zimmski/go-mutesting panics before it mutates anything, so the fork is not interchangeable")
	}
}

func TestMiseMutationTaskMutatesACopyOfTheSources(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	_, rest, found := strings.Cut(string(data), "[tasks.mutate]")
	if !found {
		t.Fatal("mise.toml defines no mutate task")
	}
	task, _, _ := strings.Cut(rest, "\n[tasks.")

	for _, want := range []string{"mktemp -d", "git ls-files", `cd "$snapshot"`} {
		if !strings.Contains(task, want) {
			t.Errorf("the mutate task does not contain %q: every mutant is written over the file it came from, so a run in the checkout leaves mutated source behind the moment it is interrupted", want)
		}
	}
}

func TestMiseMutationTaskClassifiesEveryMutant(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "mise.toml"))
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}
	_, rest, found := strings.Cut(string(data), "[tasks.mutate]")
	if !found {
		t.Fatal("mise.toml defines no mutate task")
	}
	task, _, _ := strings.Cut(rest, "\n[tasks.")

	if !strings.Contains(task, "scripts/mutate-exec.sh") {
		t.Error("the mutate task uses the built-in runner, which reads any non-zero go test status as a kill: a mutant that never compiled and a mutant that hung both count towards the score")
	}
	for _, want := range []string{"EQUIVALENT=", "HUNG="} {
		if !strings.Contains(task, want) {
			t.Errorf("the mutate task records no %s count, so it can never pass", want)
		}
	}
	for _, want := range []string{"lower it", "no mutation score was reported"} {
		if !strings.Contains(task, want) {
			t.Errorf("the mutate task does not fail on %q: a count nobody has to lower hides the next survivor, and go-mutesting exits 0 even when it mutates nothing", want)
		}
	}
}

func TestMutationExecSeparatesAHangFromAFailure(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "mutate-exec.sh"))
	if err != nil {
		t.Fatalf("read scripts/mutate-exec.sh: %v", err)
	}
	script := string(data)

	for _, want := range []string{"test timed out", "[build failed]", "exit 3", "exit 2"} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/mutate-exec.sh does not distinguish %q, which go-mutesting would otherwise count as a mutant the tests killed", want)
		}
	}
	if !strings.Contains(script, "MUTATE_TIMEOUT") {
		t.Error("scripts/mutate-exec.sh ignores MUTATE_TIMEOUT, so a mutant that loops forever never returns")
	}
}

func TestReadmeCoverageBadgeIsFedByCI(t *testing.T) {
	root := repoRoot(t)

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "https://codecov.io/gh/mach4-braai/hum/branch/master/graph/badge.svg") {
		t.Error("README.md carries no Codecov badge for master, so a reader cannot see coverage without opening a workflow run")
	}

	workflow := readWorkflow(t)
	if !strings.Contains(workflow, "codecov/codecov-action@") {
		t.Error("the CI workflow uploads nothing to Codecov, so the README badge would show a figure that never updates")
	}
	if !strings.Contains(workflow, "files: coverage.out") {
		t.Error("the Codecov step does not name coverage.out, so it would upload whatever it happens to discover rather than the profile the gate measured")
	}
}
