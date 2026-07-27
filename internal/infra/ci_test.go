//go:build infra

package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The workflow is asserted as text rather than parsed: adding a YAML library
// solely for a test would spend one of the two third-party dependencies PRD.md
// section 22 allows the whole project.
func readWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	return string(data)
}

// The audio backend is platform-sensitive, so both supported platforms must be
// covered before any audio code lands. Verified empirically: oto/v3 v3.4.0
// builds CGO-free on macOS but its driver_unix.go declares
// "#cgo pkg-config: alsa", so a CGO_ENABLED=0 Linux build fails.
func TestCIRunsOnBothSupportedPlatforms(t *testing.T) {
	workflow := readWorkflow(t)

	for _, runner := range []string{"ubuntu-latest", "macos-latest"} {
		if !strings.Contains(workflow, runner) {
			t.Errorf("ci workflow does not run on %s", runner)
		}
	}
}

// CI must take its Go toolchain from mise.toml rather than declaring its own
// version, otherwise the pin in mise.toml stops being the single source of
// truth and CI silently drifts from local builds.
func TestCIDerivesToolchainFromMise(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "jdx/mise-action") {
		t.Error("ci workflow does not install the toolchain with jdx/mise-action")
	}
	if strings.Contains(workflow, "actions/setup-go") {
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

// oto/v3 v3.4.0 links ALSA on Linux via cgo, so the Linux leg needs the
// development headers. The step is expected to disappear with the oto v3.5
// upgrade, so it must carry a comment tying it to that issue.
func TestCIInstallsLinuxAudioBuildDependency(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "libasound2-dev") {
		t.Error("ci workflow does not install libasound2-dev for the Linux leg")
	}
	if !strings.Contains(workflow, "runner.os == 'Linux'") {
		t.Error("the audio build dependency is not gated to the Linux runner")
	}
}

// A workflow that only runs on pull_request leaves the default branch
// unverified after a direct push or a squash merge, which is exactly when a
// broken main branch goes unnoticed.
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

// The Linux audio dependency disappears with the oto v3.5 upgrade. Without a
// pointer to that issue, a future maintainer has no way to know the step is
// removable and it becomes permanent.
func TestCILinuxAudioStepReferencesItsRemovalIssue(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "#39") {
		t.Error("the Linux audio dependency step does not reference issue #39, which removes it")
	}
}
