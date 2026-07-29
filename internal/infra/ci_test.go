package infra

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func readWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	return string(data)
}

func TestCIRunsOnBothSupportedPlatforms(t *testing.T) {
	workflow := readWorkflow(t)

	for _, runner := range []string{"ubuntu-latest", "macos-latest"} {
		if !strings.Contains(workflow, runner) {
			t.Errorf("ci workflow does not run on %s", runner)
		}
	}
}

func TestCIDerivesToolchainFromMise(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "jdx/mise-action") {
		t.Error("ci workflow does not install the toolchain with jdx/mise-action")
	}
	if strings.Contains(workflow, "uses: actions/setup-go") {
		t.Error("ci workflow uses actions/setup-go, which declares a Go version independently of mise.toml")
	}
	if !strings.Contains(workflow, "mise run check") {
		t.Error("ci workflow does not run `mise run check`")
	}
}

func TestCICancelsSupersededRuns(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "concurrency:") {
		t.Error("ci workflow declares no concurrency group")
	}
	if !strings.Contains(workflow, "cancel-in-progress: true") {
		t.Error("ci workflow does not cancel superseded runs")
	}
}

func TestCIInstallsLinuxAudioBuildDependency(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "libasound2-dev") {
		t.Error("ci workflow does not install libasound2-dev for the Linux leg")
	}
	if !strings.Contains(workflow, "runner.os == 'Linux'") {
		t.Error("the audio build dependency is not gated to the Linux runner")
	}
}

func TestCIRunsOnPushAndPullRequest(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "pull_request:") {
		t.Error("ci workflow does not trigger on pull_request")
	}
	if !strings.Contains(workflow, "push:") {
		t.Error("ci workflow does not trigger on push")
	}
	if !strings.Contains(workflow, "master") {
		t.Error("the push trigger is not scoped to the default branch (master)")
	}
}

func TestCICachesGoModules(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "actions/cache") {
		t.Error("ci workflow does not cache anything")
	}
	if !strings.Contains(workflow, "go/pkg/mod") {
		t.Error("ci workflow does not cache the Go module directory")
	}
}

func TestCIInstallsPkgConfigForCgoAlsa(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "pkg-config") {
		t.Error("ci workflow does not install pkg-config, required to resolve the cgo alsa directive")
	}
}

func TestCILinuxAudioStepReferencesItsRemovalIssue(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "#39") {
		t.Error("the Linux audio dependency step does not reference issue #39, which removes it")
	}
}

var pinnedActions = map[string]struct {
	version  string
	minMajor int
	commit   string
}{
	"actions/cache":                   {"v6.1.0", 5, "55cc8345863c7cc4c66a329aec7e433d2d1c52a9"},
	"actions/checkout":                {"v7.0.1", 5, "3d3c42e5aac5ba805825da76410c181273ba90b1"},
	"actions/create-github-app-token": {"v3.2.0", 3, "bcd2ba49218906704ab6c1aa796996da409d3eb1"},
	"jdx/mise-action":                 {"v4.2.3", 4, "9e7f7633ff6f6d6048a9418a68d48f288f50eb14"},
}

func TestWorkflowsPinEveryActionToAReviewedCommit(t *testing.T) {
	uses := regexp.MustCompile(`uses:\s*([\w.-]+/[\w.-]+)@(\S+)`)
	seen := make(map[string]bool, len(pinnedActions))

	var workflows []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(repoRoot(t), ".github", "workflows", pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		workflows = append(workflows, matches...)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflows found, so this asserts nothing")
	}

	for _, path := range workflows {
		file := filepath.Base(path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range uses.FindAllStringSubmatch(string(data), -1) {
			action, ref := m[1], m[2]
			pin, tracked := pinnedActions[action]
			if !tracked {
				t.Errorf("%s uses %s, which pinnedActions does not record", file, action)
				continue
			}
			seen[action] = true
			if ref != pin.commit {
				t.Errorf("%s pins %s at %s, want %s (%s): a tag can be moved onto code nobody reviewed", file, action, ref, pin.commit, pin.version)
			}
		}
	}

	for action, pin := range pinnedActions {
		if !seen[action] {
			t.Errorf("no workflow uses %s; drop it from pinnedActions or restore it", action)
			continue
		}
		major, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(pin.version, "v"), ".", 2)[0])
		if err != nil {
			t.Fatalf("parse major of %s %s: %v", action, pin.version, err)
		}
		if major < pin.minMajor {
			t.Errorf("%s is recorded at %s, want at least v%d: earlier majors run on the deprecated node20 runtime", action, pin.version, pin.minMajor)
		}
	}
}

func TestCIReportsCoverageAsASeparateStatusCheck(t *testing.T) {
	workflow := readWorkflow(t)

	job := regexp.MustCompile(`(?m)^  coverage:$`)
	if !job.MatchString(workflow) {
		t.Fatal("ci workflow declares no coverage job, so coverage cannot be a status check of its own")
	}
	if !strings.Contains(workflow, "mise run coverage") {
		t.Error("the coverage job does not run `mise run coverage`, so the minimum is enforced nowhere")
	}
}

func TestCIPublishesTheMeasuredCoverageTotal(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "context=coverage/total") {
		t.Error("the coverage job publishes no commit status, so the pull request shows a bare pass or fail")
	}
	if !strings.Contains(workflow, "statuses: write") {
		t.Error("the coverage job cannot post a status without statuses: write")
	}
	if !strings.Contains(workflow, "continue-on-error: true") {
		t.Error("a failed status post would fail the required coverage job")
	}
	if !strings.Contains(workflow, "head.repo.full_name == github.repository") {
		t.Error("the status post is not skipped for forks, whose token cannot write statuses")
	}
}
