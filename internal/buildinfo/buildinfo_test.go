package buildinfo

import (
	"encoding/json"
	"errors"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func stageBuildInfo(t *testing.T, info *debug.BuildInfo, ok bool) {
	t.Helper()
	previous := readBuildInfo
	t.Cleanup(func() { readBuildInfo = previous })
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, ok }
}

func stamped(version string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main:     debug.Module{Version: version},
		Settings: settings,
	}
}

func TestLdflagsWinOverBuildInfo(t *testing.T) {
	stageBuildInfo(t, stamped("v9.9.9",
		debug.BuildSetting{Key: "vcs.revision", Value: "ffffffffffffffff"},
		debug.BuildSetting{Key: "vcs.time", Value: "2000-01-01T00:00:00Z"},
	), true)

	info := Resolve("hum", "0.1.0", "abc1234", "2026-07-28T10:00:00Z")

	if info.Version != "0.1.0" {
		t.Errorf("Version = %q, want the ldflag value 0.1.0", info.Version)
	}
	if info.Commit != "abc1234" {
		t.Errorf("Commit = %q, want the ldflag value abc1234", info.Commit)
	}
	if info.Date != "2026-07-28T10:00:00Z" {
		t.Errorf("Date = %q, want the ldflag value", info.Date)
	}
}

func TestBuildInfoFillsUnstampedFields(t *testing.T) {
	stageBuildInfo(t, stamped("v0.0.0-20260728162130-9c2e8519a150",
		debug.BuildSetting{Key: "vcs.revision", Value: "9c2e8519a15085889725064f0a5da1dce9a0413a"},
		debug.BuildSetting{Key: "vcs.time", Value: "2026-07-28T16:21:30Z"},
		debug.BuildSetting{Key: "GOARCH", Value: "ignored"},
	), true)

	info := Resolve("hum", UnknownVersion, UnknownCommit, UnknownDate)

	if info.Version != "v0.0.0-20260728162130-9c2e8519a150" {
		t.Errorf("Version = %q, want the module pseudo-version", info.Version)
	}
	if info.Commit != "9c2e8519a150" {
		t.Errorf("Commit = %q, want the revision shortened to %d characters", info.Commit, commitWidth)
	}
	if info.Date != "2026-07-28T16:21:30Z" {
		t.Errorf("Date = %q, want the vcs.time value", info.Date)
	}
}

func TestDevelVersionIsNotReported(t *testing.T) {
	stageBuildInfo(t, stamped(develVersion), true)

	if got := Resolve("hum", UnknownVersion, UnknownCommit, UnknownDate).Version; got != UnknownVersion {
		t.Errorf("Version = %q, want %q: %q is not a usable version", got, UnknownVersion, develVersion)
	}
}

func TestEmptyModuleVersionIsNotReported(t *testing.T) {
	stageBuildInfo(t, stamped(""), true)

	if got := Resolve("hum", UnknownVersion, UnknownCommit, UnknownDate).Version; got != UnknownVersion {
		t.Errorf("Version = %q, want %q rather than an empty string", got, UnknownVersion)
	}
}

func TestMissingBuildInfoLeavesSentinels(t *testing.T) {
	stageBuildInfo(t, nil, false)

	info := Resolve("hum", UnknownVersion, UnknownCommit, UnknownDate)

	if info.Version != UnknownVersion || info.Commit != UnknownCommit || info.Date != UnknownDate {
		t.Errorf("Resolve = %+v, want the unknown sentinels preserved", info)
	}
}

func TestEmptyStringsBecomeSentinels(t *testing.T) {
	stageBuildInfo(t, nil, false)

	info := Resolve("hum", "", "", "")

	if info.Version != UnknownVersion || info.Commit != UnknownCommit || info.Date != UnknownDate {
		t.Errorf("Resolve = %+v, want empty ldflags reported as the unknown sentinels", info)
	}
}

func TestShortRevisionIsNotTruncated(t *testing.T) {
	stageBuildInfo(t, stamped("v1.0.0",
		debug.BuildSetting{Key: "vcs.revision", Value: "abc123"},
	), true)

	if got := Resolve("hum", "1.0.0", UnknownCommit, UnknownDate).Commit; got != "abc123" {
		t.Errorf("Commit = %q, want the full short revision abc123", got)
	}
}

func TestRuntimeFieldsDescribeThisBinary(t *testing.T) {
	info := Resolve("humd", "1.0.0", "abc", "date")

	if info.Go != runtime.Version() || info.OS != runtime.GOOS || info.Arch != runtime.GOARCH {
		t.Errorf("Resolve = %+v, want the runtime's own Go version and platform", info)
	}
}

func TestLineCarriesEveryField(t *testing.T) {
	info := Info{
		Program: "hum", Version: "0.1.0", Commit: "abc1234", Date: "2026-07-28T10:00:00Z",
		Go: "go1.24.13", OS: "darwin", Arch: "arm64",
	}

	want := "hum 0.1.0 (abc1234, built 2026-07-28T10:00:00Z, go1.24.13, darwin/arm64)"
	if got := info.Line(); got != want {
		t.Errorf("Line() = %q, want %q", got, want)
	}
}

func TestWriteJSONParsesBackToTheSameInfo(t *testing.T) {
	info := Resolve("hum", "0.1.0", "abc1234", "2026-07-28T10:00:00Z")

	var out strings.Builder
	if err := info.Write(&out, true); err != nil {
		t.Fatalf("Write(json) = %v, want no error", err)
	}

	var decoded Info
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("hum version --json did not parse: %v (%q)", err, out.String())
	}
	if decoded != info {
		t.Errorf("decoded = %+v, want %+v", decoded, info)
	}
}

func TestWriteTextEndsWithOneNewline(t *testing.T) {
	info := Resolve("hum", "0.1.0", "abc1234", "2026-07-28T10:00:00Z")

	var out strings.Builder
	if err := info.Write(&out, false); err != nil {
		t.Fatalf("Write(text) = %v, want no error", err)
	}

	if got := out.String(); got != info.Line()+"\n" {
		t.Errorf("Write(text) = %q, want the line plus a newline", got)
	}
}

type failingWriter struct{}

var errWriteFailed = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestWriteReportsWriterFailures(t *testing.T) {
	info := Resolve("hum", "0.1.0", "abc1234", "2026-07-28T10:00:00Z")

	for _, asJSON := range []bool{false, true} {
		if err := info.Write(failingWriter{}, asJSON); !errors.Is(err, errWriteFailed) {
			t.Errorf("Write(asJSON=%t) = %v, want the writer's error", asJSON, err)
		}
	}
}
