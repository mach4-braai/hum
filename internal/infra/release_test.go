package infra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var stampSymbols = []string{"-X main.version=", "-X main.commit=", "-X main.date="}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(data)
}

func TestGoreleaserStampsTheSameSymbolsAsMise(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	for _, symbol := range stampSymbols {
		if !strings.Contains(release, symbol) {
			t.Errorf(".goreleaser.yaml does not inject %q, so a released binary would not match a mise build", symbol)
		}
	}
	if !strings.Contains(release, "-trimpath") {
		t.Error(".goreleaser.yaml does not pass -trimpath, so released paths would differ from a mise build")
	}
}

func TestFormulaStampsTheSameSymbolsAsMise(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	for _, symbol := range stampSymbols {
		if !strings.Contains(formula, symbol) {
			t.Errorf("Formula/hum.rb does not inject %q, so a Homebrew build would misreport its version", symbol)
		}
	}
	if !strings.Contains(formula, "std_go_args") {
		t.Error("Formula/hum.rb does not use std_go_args, which is what supplies -trimpath")
	}
}

func TestFormulaDoesNotRequireMise(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	if strings.Contains(formula, "mise run") {
		t.Error("Formula/hum.rb invokes mise, which Homebrew does not provide")
	}
	if !strings.Contains(formula, `depends_on "go" => :build`) {
		t.Error("Formula/hum.rb does not declare Go as a build dependency")
	}
}

func TestFormulaInstallsBothBinaries(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	for _, target := range []string{"./cmd/hum", "./cmd/humd"} {
		if !strings.Contains(formula, target) {
			t.Errorf("Formula/hum.rb does not build %s", target)
		}
	}
}

func TestFormulaGatesAlsaDependenciesOnLinux(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	start := strings.Index(formula, "on_linux do")
	if start < 0 {
		t.Fatal("Formula/hum.rb has no on_linux block; ALSA must not be a macOS dependency")
	}
	block := formula[start:]
	if end := strings.Index(block, "\n  end"); end >= 0 {
		block = block[:end]
	}

	for _, want := range []string{"alsa-lib", "pkg-config"} {
		if !strings.Contains(block, want) {
			t.Errorf("Formula/hum.rb does not depend on %q inside on_linux", want)
		}
	}
}

func TestFormulaServiceSurvivesACleanStop(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	if !strings.Contains(formula, "service do") {
		t.Fatal("Formula/hum.rb has no service block, so brew services cannot run humd")
	}
	if !strings.Contains(formula, "keep_alive crashed: true") {
		t.Error("Formula/hum.rb does not restrict keep_alive to crashes; a clean `hum stop` would be resurrected")
	}
	if strings.Contains(formula, "keep_alive true") {
		t.Error("Formula/hum.rb keeps the service alive unconditionally, which defeats `hum stop`")
	}
	for _, want := range []string{"log_path", "error_log_path", "working_dir"} {
		if !strings.Contains(formula, want) {
			t.Errorf("Formula/hum.rb service block does not set %s", want)
		}
	}
}

func TestReleaseWorkflowGatesOnTheCheckTask(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	if !strings.Contains(workflow, "mise run check") {
		t.Fatal("release.yml does not run mise run check, so a failing test could not block a release")
	}
	if !strings.Contains(workflow, "needs: check") {
		t.Error("release.yml does not make the release job depend on the check job")
	}
	if !strings.Contains(workflow, `tags: ["v*"]`) {
		t.Error("release.yml does not trigger on v* tags")
	}
}

func TestReleaseWorkflowInstallsGoreleaser(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")
	manifest := readRepoFile(t, "mise.toml")

	if !strings.Contains(workflow, "goreleaser release") {
		t.Fatal("release.yml does not invoke goreleaser")
	}
	if !strings.Contains(workflow, "jdx/mise-action") {
		t.Error("release.yml does not set up mise, which is what installs goreleaser")
	}
	if !strings.Contains(manifest, "goreleaser =") {
		t.Error("mise.toml does not pin goreleaser under [tools], so the release job would not have the binary")
	}
}

func TestReleaseWorkflowBuildsLinuxArtefacts(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	if !strings.Contains(workflow, "HUM_RELEASE_LINUX") {
		t.Fatal("release.yml does not enable the Linux builds, which are skipped by default")
	}
	for _, want := range []string{"libasound2-dev", "aarch64-linux-gnu"} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release.yml does not install %q, so the cgo Linux builds would fail", want)
		}
	}
}

func TestGoreleaserSkipsLinuxUnlessEnabled(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	if !strings.Contains(release, `envOrDefault "HUM_RELEASE_LINUX" "0"`) {
		t.Error(".goreleaser.yaml does not gate the cgo Linux builds, so a macOS snapshot could not succeed")
	}
}

func TestGoreleaserShipsChecksumsAndSource(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	if !strings.Contains(release, "checksums.txt") {
		t.Error(".goreleaser.yaml does not emit checksums.txt")
	}
	source := strings.Index(release, "\nsource:")
	if source < 0 || !strings.Contains(release[source:], "enabled: true") {
		t.Error(".goreleaser.yaml does not emit a source archive, which the formula builds from")
	}
}

func TestGoreleaserArchivesShipLicenceAndDocs(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	for _, want := range []string{"LICENSE", "README.md", "docs/"} {
		if !strings.Contains(release, want) {
			t.Errorf(".goreleaser.yaml archives do not include %q", want)
		}
	}
}

func TestGoreleaserExcludesDocsAndChoresFromTheChangelog(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	for _, want := range []string{`'^docs(\([^)]*\))?!?:'`, `'^chore(\([^)]*\))?!?:'`} {
		if !strings.Contains(release, want) {
			t.Errorf(".goreleaser.yaml changelog does not exclude %s", want)
		}
	}
}

func TestGoreleaserExcludesWindowsArm64(t *testing.T) {
	release := readRepoFile(t, ".goreleaser.yaml")

	if !strings.Contains(release, "goos: windows") || !strings.Contains(release, "goarch: arm64") {
		t.Error(".goreleaser.yaml does not ignore windows/arm64, which has no supported audio backend")
	}
}

func TestMiseDefinesTheSnapshotTask(t *testing.T) {
	manifest := readRepoFile(t, "mise.toml")

	if !strings.Contains(manifest, "[tasks.snapshot]") {
		t.Error("mise.toml has no snapshot task, so the local release check is not reproducible")
	}
}

func TestReleaseWorkflowNeedsNoCrossRepositoryCredential(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	for _, forbidden := range []string{"TAP_GITHUB_TOKEN", "x-access-token", "homebrew-tap.git"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("release.yml references %q; pushing to the tap from here needs a long-lived secret in a public repository", forbidden)
		}
	}
	if found := secretsIn(workflow); len(found) != 0 {
		t.Errorf("release.yml references secrets.%v; releasing must need no user-managed secret, so use github.token", found)
	}
}

func secretsIn(workflow string) []string {
	var found []string
	for _, part := range strings.Split(workflow, "secrets.")[1:] {
		end := strings.IndexFunc(part, func(r rune) bool {
			return !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
		})
		if end < 0 {
			end = len(part)
		}
		found = append(found, part[:end])
	}
	return found
}

func TestTapBumpWorkflowIsSelfContained(t *testing.T) {
	workflow := readRepoFile(t, "contrib", "homebrew-tap", "bump-formula.yml")

	if found := secretsIn(workflow); len(found) != 0 {
		t.Errorf("the tap bump workflow references secrets.%v; it must need no user-managed secret, so use github.token", found)
	}
	if !strings.Contains(workflow, "contents: write") {
		t.Error("the tap bump workflow cannot commit without contents: write")
	}
	if !strings.Contains(workflow, "workflow_dispatch") {
		t.Error("the tap bump workflow has no manual trigger, so a release cannot be picked up on demand")
	}
	if !strings.Contains(workflow, "checksums.txt") {
		t.Error("the tap bump workflow does not read the published checksum, so the formula could disagree with the release")
	}
}

func TestSecretsInFindsEveryReference(t *testing.T) {
	workflow := "a: ${{ secrets.GITHUB_TOKEN }}\nb: ${{ secrets.TAP_GITHUB_TOKEN }}\nc: ${{secrets.DEPLOY_KEY}}\n"

	found := secretsIn(workflow)

	want := []string{"GITHUB_TOKEN", "TAP_GITHUB_TOKEN", "DEPLOY_KEY"}
	if len(found) != len(want) {
		t.Fatalf("secretsIn = %v, want %v", found, want)
	}
	for i, name := range want {
		if found[i] != name {
			t.Errorf("secretsIn()[%d] = %q, want %q", i, found[i], name)
		}
	}
}

func TestSecretsInReportsNothingWhenThereAreNoSecrets(t *testing.T) {
	if found := secretsIn("steps:\n  - run: make\n"); len(found) != 0 {
		t.Errorf("secretsIn = %v, want none", found)
	}
}
