package paths

import (
	"errors"
	"path/filepath"
	"testing"
)

func stubExecutable(t *testing.T, path string, err error) {
	t.Helper()
	original := executable
	t.Cleanup(func() { executable = original })
	executable = func() (string, error) { return path, err }
}

func TestLogFileDerivesThePrefixFromEveryInstallShape(t *testing.T) {
	for name, exe := range map[string]string{
		"linked":  "/opt/homebrew/bin/hum",
		"cellar":  "/opt/homebrew/Cellar/hum/0.1.3/bin/hum",
		"opt":     "/opt/homebrew/opt/hum/bin/hum",
		"install": "/usr/local/bin/hum",
	} {
		t.Run(name, func(t *testing.T) {
			stubExecutable(t, exe, nil)

			log, ok := LogFile()

			if !ok {
				t.Fatalf("LogFile() reported no path for %s", exe)
			}
			want := filepath.Join(prefixOf(exe), "var", "log", "hum", LogFileName)
			if log != want {
				t.Errorf("LogFile() = %q, want %q", log, want)
			}
		})
	}
}

func prefixOf(exe string) string {
	switch exe {
	case "/usr/local/bin/hum":
		return "/usr/local"
	default:
		return "/opt/homebrew"
	}
}

func TestLogFileReportsNothingOutsideABinDirectory(t *testing.T) {
	stubExecutable(t, "/tmp/build/hum", nil)

	if log, ok := LogFile(); ok {
		t.Errorf("LogFile() = %q, %v, want no path: a binary outside bin/ has no prefix to derive", log, ok)
	}
}

func TestLogFileReportsNothingWhenTheExecutableIsUnknown(t *testing.T) {
	stubExecutable(t, "", errors.New("no executable"))

	if log, ok := LogFile(); ok {
		t.Errorf("LogFile() = %q, %v, want no path", log, ok)
	}
}
