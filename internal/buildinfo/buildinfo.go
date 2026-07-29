package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

const (
	UnknownVersion = "dev"
	UnknownCommit  = "none"
	UnknownDate    = "unknown"
)

const develVersion = "(devel)"

const commitWidth = 12

type Info struct {
	Program string `json:"program"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

var readBuildInfo = debug.ReadBuildInfo

func Resolve(program, version, commit, date string) Info {
	info := Info{
		Program: program,
		Version: fallback(version, UnknownVersion),
		Commit:  fallback(commit, UnknownCommit),
		Date:    fallback(date, UnknownDate),
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	stamped, ok := readBuildInfo()
	if !ok {
		return info
	}

	if info.Version == UnknownVersion && stamped.Main.Version != "" && stamped.Main.Version != develVersion {
		info.Version = stamped.Main.Version
	}
	for _, setting := range stamped.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == UnknownCommit {
				info.Commit = shorten(setting.Value)
			}
		case "vcs.time":
			if info.Date == UnknownDate {
				info.Date = setting.Value
			}
		}
	}
	return info
}

func fallback(value, whenEmpty string) string {
	if value == "" {
		return whenEmpty
	}
	return value
}

func shorten(revision string) string {
	if len(revision) <= commitWidth {
		return revision
	}
	return revision[:commitWidth]
}

func (i Info) Line() string {
	return fmt.Sprintf("%s %s (%s, built %s, %s, %s/%s)",
		i.Program, i.Version, i.Commit, i.Date, i.Go, i.OS, i.Arch)
}

func (i Info) Write(w io.Writer, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(i)
	}
	_, err := fmt.Fprintln(w, i.Line())
	return err
}
