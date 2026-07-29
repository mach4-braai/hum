package infra

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/paths"
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

func TestReleaseWorkflowPinsTheTagGoreleaserBuilds(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	if !strings.Contains(workflow, "GORELEASER_CURRENT_TAG: ${{ github.ref_name }}") {
		t.Error("release.yml lets goreleaser choose among the tags pointing at HEAD, so promoting a candidate would rebuild the candidate")
	}
}

func TestReleaseWorkflowBuildsLinuxArtefacts(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")

	if !strings.Contains(workflow, "HUM_RELEASE_LINUX") {
		t.Fatal("release.yml does not enable the Linux builds, which are skipped by default")
	}
	action := readRepoFile(t, ".github", "actions", "linux-packages", "action.yml")
	for _, want := range []string{"libasound2-dev", "libasound2-dev:arm64", "gcc-aarch64-linux-gnu"} {
		if !strings.Contains(action, want) {
			t.Errorf("the linux-packages action does not install %q, so the cgo Linux builds would fail", want)
		}
	}
}

const linuxPackages = "./.github/actions/linux-packages"

func TestLinuxPackagesComeFromACache(t *testing.T) {
	action := readRepoFile(t, ".github", "actions", "linux-packages", "action.yml")

	if !strings.Contains(action, "--download-only") || !strings.Contains(action, "dpkg -i") {
		t.Fatal("the packages are installed straight from the mirror, so a cache hit would save nothing")
	}
	if !strings.Contains(action, "key: ${{ steps.image.outputs.key }}") {
		t.Error("the cache key is not built by a shell step, and ImageVersion is a runner process variable the env context need not carry: an empty expression collapses the key")
	}
	if !strings.Contains(action, "${ImageVersion:-run-$GITHUB_RUN_ID}") {
		t.Error("a missing ImageVersion must degrade to a cache miss; sharing one key across image revisions installs exact-version debs that dpkg cannot reconcile")
	}
	if !strings.Contains(action, "steps.packages.outputs.cache-hit != 'true'") {
		t.Error("the action updates the package indices unconditionally, which is the network round trip the cache exists to skip")
	}
}

func TestTheDefaultBranchWarmsThePackageCache(t *testing.T) {
	release := readRepoFile(t, ".github", "workflows", "release.yml")

	if strings.Count(release, linuxPackages) < 2 {
		t.Fatalf("release.yml uses %s once: a run on a tag cannot restore a cache created for another tag, so the snapshot job on the default branch is what warms it", linuxPackages)
	}
	if !strings.Contains(release, "if: github.ref == 'refs/heads/master'") {
		t.Error("no job is restricted to the default branch, which is the only scope a tag run can restore a cache from")
	}
	if !strings.Contains(release, "mise run snapshot") {
		t.Error("the default branch builds no snapshot, so nothing exercises the release path until a tag is cut")
	}
	if strings.Count(release, "goreleaser release") != 1 {
		t.Error("release.yml invokes a publishing goreleaser more than once; a push to the default branch must build artefacts and release nothing")
	}
}

func TestReleaseJobsRunOnlyForTags(t *testing.T) {
	release := readRepoFile(t, ".github", "workflows", "release.yml")

	if strings.Count(release, "if: startsWith(github.ref, 'refs/tags/v')") != 2 {
		t.Error("the check and release jobs are not both gated on a tag, so a push to the default branch would publish or double-run the suite ci.yml already runs")
	}
}

func TestWorkflowsSurviveTheZizmorAudits(t *testing.T) {
	for _, file := range []string{"ci.yml", "promote.yml", "release.yml"} {
		workflow := readRepoFile(t, ".github", "workflows", file)

		checkouts := strings.Count(workflow, "uses: actions/checkout@")
		guarded := strings.Count(workflow, "persist-credentials: false") + strings.Count(workflow, "zizmor: ignore[artipacked]")
		if checkouts != guarded {
			t.Errorf("%s has %d checkouts but %d that say what becomes of the token: artipacked persists it for everything later in the job", file, checkouts, guarded)
		}

		_, body, found := strings.Cut(workflow, "\njobs:\n")
		if !found {
			t.Fatalf("%s declares no jobs", file)
		}
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			id, isJob := strings.CutSuffix(line, ":")
			if !isJob || !strings.HasPrefix(id, "  ") || strings.HasPrefix(id, "   ") {
				continue
			}
			id = strings.TrimPrefix(id, "  ")
			if want := "    name: " + id; i+1 >= len(lines) || lines[i+1] != want {
				t.Errorf("%s job %q is not followed by %q: zizmor --pedantic reports an unnamed job, and GitHub derives the status check the master ruleset requires from the id", file, id, want)
			}
		}
	}

	release := readRepoFile(t, ".github", "workflows", "release.yml")
	if strings.Count(release, "uses: jdx/mise-action@") != strings.Count(release, "cache: false") {
		t.Error("a mise step in release.yml caches by default, and a cache restored into the job that publishes is the cache-poisoning path zizmor flags")
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

func TestNeitherWorkflowUsesALongLivedCrossRepositoryCredential(t *testing.T) {
	release := readRepoFile(t, ".github", "workflows", "release.yml")
	promote := readRepoFile(t, ".github", "workflows", "promote.yml")

	for _, forbidden := range []string{
		"TAP_GITHUB_TOKEN", "PERSONAL_ACCESS_TOKEN", "_PAT",
		"DEPLOY_KEY", "SSH_PRIVATE_KEY", "x-access-token", "homebrew-tap.git",
	} {
		if strings.Contains(release+promote, forbidden) {
			t.Errorf("a workflow references %q; the tap is written with an App installation token that expires within the hour, not a long-lived secret", forbidden)
		}
	}

	if found := secretsIn(release); len(found) != 0 {
		t.Errorf("release.yml references secrets.%v; building and drafting a release needs no user-managed secret", found)
	}
	found := secretsIn(promote)
	if len(found) != 1 || found[0] != "TAP_APP_PRIVATE_KEY" {
		t.Errorf("promote.yml references secrets.%v; the App private key is the only secret it may need, and the client id belongs in vars", found)
	}
}

func TestPromoteMintsATapScopedAppToken(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "promote.yml")

	for _, required := range []string{
		"actions/create-github-app-token@",
		"client-id: ${{ vars.TAP_APP_CLIENT_ID }}",
		"private-key: ${{ secrets.TAP_APP_PRIVATE_KEY }}",
		"repositories: homebrew-tap",
		"permission-contents: write",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("promote.yml does not carry %q, so the formula bump has no credential for the tap", required)
		}
	}
}

func TestGoreleaserDraftsTheReleaseForImmutability(t *testing.T) {
	config := readRepoFile(t, ".goreleaser.yaml")

	if !strings.Contains(config, "draft: true") {
		t.Error(".goreleaser.yaml publishes directly; an immutable release is frozen the moment it is created, so every asset upload after that returns 422")
	}
	if !strings.Contains(config, "replace_existing_draft: true") {
		t.Error(".goreleaser.yaml leaves an existing draft in place, so re-running a failed release would add a second draft for the same tag beside a half-uploaded first")
	}
}

func TestPromoteRunsWhenADraftIsPublished(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "promote.yml")

	if !strings.Contains(workflow, "types: [published]") {
		t.Error("promote.yml does not subscribe to published, the one activity GitHub documents as covering publication from a draft; released and prereleased do not reliably fire for one")
	}
	if !strings.Contains(workflow, "!github.event.release.prerelease") {
		t.Error("promote.yml does not exclude prereleases, so publishing one would make it the default brew install")
	}
	if !strings.Contains(workflow, "!contains(github.event.release.tag_name, '-')") {
		t.Error("promote.yml is not gated on a stable tag name, so a candidate tag published as a release would reach the tap")
	}
	if !strings.Contains(workflow, "ref: ${{ github.event.release.tag_name }}") {
		t.Error("promote.yml does not check out the released tag, so it would rewrite the formula master happens to carry now")
	}
	if !strings.Contains(workflow, "checksums.txt") {
		t.Error("promote.yml does not read the published checksum, so the formula could disagree with the release")
	}
}

func TestTapBumpRefusesToGoBackwards(t *testing.T) {
	workflow := readRepoFile(t, ".github", "workflows", "promote.yml")

	if !strings.Contains(workflow, "group: tap") {
		t.Error("the promote workflow shares no concurrency group, so two publishes can rewrite the formula at once")
	}
	if !strings.Contains(workflow, "sort -V") {
		t.Error("the promote workflow does not compare the tap's version with the tag, so publishing an older release would downgrade the formula")
	}
}

var secretReference = regexp.MustCompile(`secrets\s*(?:\.\s*([A-Za-z_][A-Za-z0-9_]*)|\[\s*['"]?([^'"\]]+))`)

func secretsIn(workflow string) []string {
	var found []string
	for _, match := range secretReference.FindAllStringSubmatch(workflow, -1) {
		name := match[1]
		if name == "" {
			name = strings.TrimSpace(match[2])
		}
		found = append(found, name)
	}
	return found
}

func TestSecretsInFindsEveryReference(t *testing.T) {
	workflow := strings.Join([]string{
		"a: ${{ secrets.GITHUB_TOKEN }}",
		"b: ${{ secrets.TAP_GITHUB_TOKEN }}",
		"c: ${{secrets.DEPLOY_KEY}}",
		"d: ${{ secrets['BRACKET_SINGLE'] }}",
		`e: ${{ secrets["BRACKET_DOUBLE"] }}`,
		"f: ${{ secrets [ 'SPACED' ] }}",
	}, "\n")

	found := secretsIn(workflow)

	want := []string{
		"GITHUB_TOKEN", "TAP_GITHUB_TOKEN", "DEPLOY_KEY",
		"BRACKET_SINGLE", "BRACKET_DOUBLE", "SPACED",
	}
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

func TestFormulaRoutesTheDaemonLogWhereDoctorLooks(t *testing.T) {
	formula := readRepoFile(t, "Formula", "hum.rb")

	want := `error_log_path var/"log/hum/` + paths.LogFileName + `"`
	if !strings.Contains(formula, want) {
		t.Errorf("Formula/hum.rb does not carry %s; humd logs to stderr, so that is the file hum doctor reports and the only one a crash lands in", want)
	}
}
