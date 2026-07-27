package infra

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Asserted as text; a YAML parser would cost one of two allowed dependencies.
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
	// `uses:`, not any mention: the workflow names setup-go in a comment.
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

// libasound2-dev alone cannot link; pkg-config resolves the cgo directive.
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

// Minimum majors, so the warning cannot return and nobody chases each release.
func TestCIPinsActionsOnSupportedNodeRuntime(t *testing.T) {
	// mise-action v3 is also node20, so v4 is the floor, not v3.
	minMajor := map[string]int{
		"actions/checkout": 5,
		"actions/cache":    5,
		"jdx/mise-action":  4,
	}

	uses := regexp.MustCompile(`uses:\s*([\w.-]+/[\w.-]+)@v(\d+)`)
	found := make(map[string]bool, len(minMajor))
	for _, m := range uses.FindAllStringSubmatch(readWorkflow(t), -1) {
		action, major := m[1], m[2]
		found[action] = true
		got, err := strconv.Atoi(major)
		if err != nil {
			t.Fatalf("parse major of %s@v%s: %v", action, major, err)
		}
		if want, tracked := minMajor[action]; tracked && got < want {
			t.Errorf("%s is pinned at v%d, want at least v%d: earlier majors run on the deprecated node20 runtime", action, got, want)
		}
	}

	for action := range minMajor {
		if !found[action] {
			t.Errorf("workflow no longer uses %s; drop it from minMajor or restore it", action)
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
	// A fork's token is read-only, so posting must never gate the merge.
	if !strings.Contains(workflow, "continue-on-error: true") {
		t.Error("a failed status post would fail the required coverage job")
	}
	if !strings.Contains(workflow, "head.repo.full_name == github.repository") {
		t.Error("the status post is not skipped for forks, whose token cannot write statuses")
	}
}
