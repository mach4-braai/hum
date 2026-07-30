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

func jobBlock(t *testing.T, workflow, name string) string {
	t.Helper()
	_, rest, found := strings.Cut(workflow, "\n  "+name+":\n")
	if !found {
		t.Fatalf("ci.yml has no %s job", name)
	}
	if next := regexp.MustCompile(`\n  [a-z0-9-]+:\n`).FindStringIndex(rest); next != nil {
		rest = rest[:next[0]]
	}
	return rest
}

func TestCIChecksEverySupportedPlatform(t *testing.T) {
	block := jobBlock(t, readWorkflow(t), "check")

	for _, runner := range []string{"ubuntu-latest", "macos-latest", "windows-latest"} {
		if !strings.Contains(block, runner) {
			t.Errorf("the check job does not run on %s, so its archives would ship untested", runner)
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

func TestCIVerifiesTheSystemdUnit(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "systemd-analyze --user verify") {
		t.Error("ci workflow does not run systemd-analyze against contrib/systemd/humd.service, so a malformed unit would ship unnoticed")
	}
	if !strings.Contains(workflow, `PREFIX="$HOME/.local" mise run install`) {
		t.Error("the systemd check does not install to the prefix the unit's ExecStart names, so verify would only prove the file parses")
	}
}

func TestCIRunsTheAcceptanceSuiteAsItsOwnJob(t *testing.T) {
	workflow := readWorkflow(t)

	if !strings.Contains(workflow, "mise run e2e") {
		t.Fatal("ci workflow does not run the acceptance suite")
	}
	block := jobBlock(t, workflow, "e2e")
	for _, runner := range []string{"ubuntu-latest", "macos-latest"} {
		if !strings.Contains(block, runner) {
			t.Errorf("the e2e job does not run on %s", runner)
		}
	}
	if strings.Contains(block, "windows-latest") {
		t.Error("the e2e job claims Windows; the acceptance suite drives POSIX signals and has never been run there")
	}
}

func TestTheAcceptanceSuiteIsTaggedOutOfTheDefaultRun(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "e2e"))
	if err != nil {
		t.Fatalf("read e2e directory: %v", err)
	}

	files := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		files++
		data, err := os.ReadFile(filepath.Join(root, "e2e", entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if !strings.HasPrefix(string(data), "//go:build e2e\n") {
			t.Errorf("e2e/%s does not open with //go:build e2e, so `go test ./...` would spawn binaries and take minutes", entry.Name())
		}
	}
	if files == 0 {
		t.Error("the e2e directory holds no Go files")
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

const goCaches = "./.github/actions/go-cache"

func readGoCacheAction(t *testing.T) string {
	t.Helper()
	return readRepoFile(t, ".github", "actions", "go-cache", "action.yml")
}

func TestEveryCIJobRestoresTheGoCaches(t *testing.T) {
	workflow := readWorkflow(t)

	jobs := strings.Count(workflow, "runs-on:")
	if n := strings.Count(workflow, goCaches); n != jobs {
		t.Errorf("ci.yml has %d jobs but restores the Go caches %d times: every job downloads the same modules and compiles the same packages", jobs, n)
	}
	if strings.Contains(workflow, "go/pkg/mod") {
		t.Error("ci.yml caches the toolchain's default directory, which is outside the workspace and named differently on every runner")
	}
}

func cachedPath(t *testing.T, step string) string {
	t.Helper()
	_, rest, found := strings.Cut(step, "path: ")
	if !found {
		t.Fatalf("no path in cache step %q", step)
	}
	path, _, _ := strings.Cut(rest, "\n")
	return strings.TrimSpace(path)
}

func TestTheModuleCacheIsSharedByEveryRunner(t *testing.T) {
	action := readGoCacheAction(t)

	modules, _, found := strings.Cut(action, "Cache the compiler output")
	if !found {
		t.Fatal("the go-cache action has no compiler output step, so there is nothing to separate the shared cache from")
	}
	if !strings.Contains(modules, "enableCrossOsArchive: true") {
		t.Error("the module cache is written in a format only its own runner can restore, so each operating system keeps a copy of the same sources")
	}
	if strings.Contains(modules, "runner.os") {
		t.Error("the module key names the runner, which is exactly the sharing enableCrossOsArchive exists to allow")
	}

	path := cachedPath(t, modules)
	if filepath.IsAbs(path) || strings.ContainsAny(path, "~$") || strings.Contains(path, "{{") {
		t.Errorf("the module cache path is %q: actions/cache hashes the path it is given into the cache version, so anything that expands per runner gives each one a cache of its own under the same key", path)
	}
	if strings.HasPrefix(path, "..") {
		t.Errorf("the module cache path is %q: `@actions/glob` refuses a pattern that walks out of the workspace, and the step then warns and archives nothing", path)
	}
	if !strings.Contains(action, "GOMODCACHE=$GITHUB_WORKSPACE/") {
		t.Errorf("the module path %q is relative to the workspace but the toolchain still writes elsewhere, so the cache would archive nothing", path)
	}
	if strings.Contains(action, "**/go.sum") {
		t.Error("the key globs every go.sum in the tree, and a restored module cache puts one there per dependency: the key would depend on what it restored")
	}
}

func TestTheCompilerOutputCacheStaysWithItsRunner(t *testing.T) {
	action := readGoCacheAction(t)

	_, build, found := strings.Cut(action, "Cache the compiler output")
	if !found {
		t.Fatal("the go-cache action caches no compiler output")
	}
	if !strings.Contains(build, "key: ${{ runner.os }}-go-build-") {
		t.Error("object files are compiled for one GOOS and GOARCH; sharing them under one key stores a cache every runner but the first must ignore")
	}
	if strings.Contains(build, "enableCrossOsArchive") {
		t.Error("the compiler output is offered across operating systems, which pays the archive cost for entries nothing can reuse")
	}
	if !strings.Contains(action, "GOCACHE=$GITHUB_WORKSPACE/") {
		t.Error("the compiler output is left outside the workspace, where a relative cache path cannot reach it")
	}
}

func TestTheGoToolsIgnoreTheRestoredCaches(t *testing.T) {
	action := readGoCacheAction(t)
	manifest := readRepoFile(t, "mise.toml")

	dir, _, found := strings.Cut(strings.TrimPrefix(cachedPath(t, action), "./"), "/")
	if !found {
		t.Fatalf("the cache path %q has no directory to exclude", dir)
	}
	_, check, found := strings.Cut(manifest, "\n[tasks.check]")
	if !found {
		t.Fatal("mise.toml defines no check task")
	}
	check, _, _ = strings.Cut(check, "\n[tasks.")
	if !strings.Contains(check, "-path ./"+dir+" -prune") {
		t.Errorf("`mise run check` formats every .go file under the tree, and in CI %s holds the restored module cache: every dependency would be reported unformatted", dir)
	}

	_, coverage, found := strings.Cut(manifest, "\n[tasks.coverage]")
	if !found {
		t.Fatal("mise.toml defines no coverage task")
	}
	coverage, _, _ = strings.Cut(coverage, "\n[tasks.")
	if strings.Contains(coverage, "-coverpkg=./...") {
		t.Errorf("`-coverpkg` matches loaded packages by directory rather than walking one, so with the module cache in %s every dependency is instrumented and the total falls to around 54%%", dir)
	}
	if !strings.Contains(coverage, `-coverpkg="$MODULE/..."`) {
		t.Error("the coverage set is not pinned to the module's own import path, which is the only pattern the module cache cannot join")
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

	var manifests []string
	for _, pattern := range []string{
		filepath.Join(".github", "workflows", "*.yml"),
		filepath.Join(".github", "workflows", "*.yaml"),
		filepath.Join(".github", "actions", "*", "action.yml"),
		filepath.Join(".github", "actions", "*", "action.yaml"),
	} {
		matches, err := filepath.Glob(filepath.Join(repoRoot(t), pattern))
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		manifests = append(manifests, matches...)
	}
	if len(manifests) == 0 {
		t.Fatal("no workflows or actions found, so this asserts nothing")
	}

	for _, path := range manifests {
		file, err := filepath.Rel(repoRoot(t), path)
		if err != nil {
			t.Fatalf("relativise %s: %v", path, err)
		}
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
