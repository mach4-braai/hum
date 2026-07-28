package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/buildinfo"
)

func TestVersionPrintsTheBuildLine(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != build().Line() {
		t.Errorf("stdout = %q, want %q", got, build().Line())
	}
	for _, want := range []string{"hum", version} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout = %q, want it to contain %q", got, want)
		}
	}
}

func TestVersionJSONParses(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version", "--json"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}

	var decoded buildinfo.Info
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("hum version --json did not parse: %v (%q)", err, stdout.String())
	}
	if decoded != build() {
		t.Errorf("decoded = %+v, want %+v", decoded, build())
	}
}

func TestVersionRejectsAnOperand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version", "extra"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit %d, want %d (exitUsage)", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "extra") {
		t.Errorf("stderr = %q, want the rejected operand named", stderr.String())
	}
}

func TestVersionRejectsAnUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version", "--bogus"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit %d, want %d (exitUsage)", code, exitUsage)
	}
}

type brokenStdout struct{}

var errBrokenStdout = errors.New("stdout closed")

func (brokenStdout) Write([]byte) (int, error) { return 0, errBrokenStdout }

func TestVersionReportsAnUnwritableStdout(t *testing.T) {
	var stderr bytes.Buffer

	if code := run([]string{"version"}, brokenStdout{}, &stderr); code != exitDaemonError {
		t.Errorf("exit %d, want %d (exitDaemonError)", code, exitDaemonError)
	}
	if !strings.Contains(stderr.String(), errBrokenStdout.Error()) {
		t.Errorf("stderr = %q, want the write failure reported", stderr.String())
	}
}

func TestVersionIsUsageListed(t *testing.T) {
	if !strings.Contains(usage, "version") {
		t.Error("usage does not list the version command")
	}
}

func buildStamped(t *testing.T, dir, name, pkg string, ldflags string) string {
	t.Helper()
	binary := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binary, pkg)
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return binary
}

func TestBothBinariesReportIdenticalStampsFromOneBuild(t *testing.T) {
	dir := t.TempDir()
	ldflags := "-X main.version=9.9.9 -X main.commit=deadbeefcafe -X main.date=2026-07-28T00:00:00Z"

	humBinary := buildStamped(t, dir, "hum", "./hum", ldflags)
	humdBinary := buildStamped(t, dir, "humd", "./humd", ldflags)

	decode := func(binary string, args ...string) buildinfo.Info {
		out, err := exec.Command(binary, args...).Output()
		if err != nil {
			t.Fatalf("%s %v: %v", binary, args, err)
		}
		var info buildinfo.Info
		if err := json.Unmarshal(out, &info); err != nil {
			t.Fatalf("%s %v did not print JSON: %v (%q)", binary, args, err, out)
		}
		return info
	}

	client := decode(humBinary, "version", "--json")
	daemon := decode(humdBinary, "--version", "--json")

	if client.Program != "hum" || daemon.Program != "humd" {
		t.Errorf("programs = %q and %q, want hum and humd", client.Program, daemon.Program)
	}
	client.Program, daemon.Program = "", ""
	if client != daemon {
		t.Errorf("hum reported %+v but humd reported %+v from the same build", client, daemon)
	}
	if client.Version != "9.9.9" {
		t.Errorf("Version = %q, want the injected 9.9.9 rather than a build-info fallback", client.Version)
	}
	if client.Commit != "deadbeefcafe" || client.Date != "2026-07-28T00:00:00Z" {
		t.Errorf("stamps = %q and %q, want the injected commit and date", client.Commit, client.Date)
	}
}

func TestUnstampedBuildReportsAModuleVersion(t *testing.T) {
	dir := t.TempDir()

	binary := buildStamped(t, dir, "hum", "./hum", "")

	out, err := exec.Command(binary, "version", "--json").Output()
	if err != nil {
		t.Fatalf("hum version: %v", err)
	}
	var info buildinfo.Info
	if err := json.Unmarshal(out, &info); err != nil {
		t.Fatalf("did not print JSON: %v (%q)", err, out)
	}

	if info.Version == buildinfo.UnknownVersion {
		t.Errorf("Version = %q, want a module version recovered from build info", info.Version)
	}
	if info.Commit == buildinfo.UnknownCommit {
		t.Errorf("Commit = %q, want the vcs revision recovered from build info", info.Commit)
	}
}

func TestMiseBuildStampsEveryField(t *testing.T) {
	manifest, err := os.ReadFile("../../mise.toml")
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}

	for _, symbol := range []string{"-X main.version=", "-X main.commit=", "-X main.date="} {
		if !strings.Contains(string(manifest), symbol) {
			t.Errorf("mise.toml does not inject %s, so a built binary cannot report it", symbol)
		}
	}
}
