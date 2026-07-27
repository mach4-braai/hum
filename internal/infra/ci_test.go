package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Asserted as text: a YAML library would spend one of the two dependencies
// PRD.md section 22 allows the whole project.
func readWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	return string(data)
}

// Both platforms are covered because oto/v3 v3.4.0 builds CGO-free on macOS but
// declares "#cgo pkg-config: alsa" on Linux, where CGO_ENABLED=0 fails.
func TestCIRunsOnBothSupportedPlatforms(t *testing.T) {
	workflow := readWorkflow(t)

	for _, runner := range []string{"ubuntu-latest", "macos-latest"} {
		if !strings.Contains(workflow, runner) {
			t.Errorf("ci workflow does not run on %s", runner)
		}
	}
}

// CI must take its toolchain from mise.toml, or the pin stops being the single
// source of truth and CI drifts from local builds.
func TestCIDerivesToolchainFromMise(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "jdx/mise-action") {
		t.Error("ci workflow does not install the toolchain with jdx/mise-action")
	}
	// Matches actual usage rather than any mention: a comment explaining why
	// setup-go is avoided is desirable, and must not trip this assertion.
	if strings.Contains(workflow, "uses: actions/setup-go") {
		t.Error("ci workflow uses actions/setup-go, which declares a Go version independently of mise.toml")
	}
	if !strings.Contains(workflow, "mise run check") {
		t.Error("ci workflow does not run `mise run check`")
	}
}

// Without cancel-in-progress, pushing twice to a branch leaves both runs
// competing for runners and the stale one can report after the fresh one.
func TestCICancelsSupersededRuns(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "concurrency:") {
		t.Error("ci workflow declares no concurrency group")
	}
	if !strings.Contains(workflow, "cancel-in-progress: true") {
		t.Error("ci workflow does not cancel superseded runs")
	}
}

// oto/v3 v3.4.0 links ALSA through cgo on Linux, so the headers are needed.
func TestCIInstallsLinuxAudioBuildDependency(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "libasound2-dev") {
		t.Error("ci workflow does not install libasound2-dev for the Linux leg")
	}
	if !strings.Contains(workflow, "runner.os == 'Linux'") {
		t.Error("the audio build dependency is not gated to the Linux runner")
	}
}

// pull_request alone leaves the default branch unverified after a direct push.
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

// Without a module cache every job re-downloads the dependency graph, which
// grows into minutes of runner time once oto and yaml.v3 are in go.mod.
func TestCICachesGoModules(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "actions/cache") {
		t.Error("ci workflow does not cache anything")
	}
	if !strings.Contains(workflow, "go/pkg/mod") {
		t.Error("ci workflow does not cache the Go module directory")
	}
}

// pkg-config is what resolves the "#cgo pkg-config: alsa" directive in
// oto/v3 v3.4.0; libasound2-dev alone is not enough to link.
func TestCIInstallsPkgConfigForCgoAlsa(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "pkg-config") {
		t.Error("ci workflow does not install pkg-config, required to resolve the cgo alsa directive")
	}
}

// Without a pointer to the issue that removes it, the step becomes permanent.
func TestCILinuxAudioStepReferencesItsRemovalIssue(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "#39") {
		t.Error("the Linux audio dependency step does not reference issue #39, which removes it")
	}
}
