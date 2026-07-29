package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const EnvHome = "HUM_HOME"
const ConfigFileName = "config.yaml"
const ProjectDirName = ".hum"
const EnvSocket = "HUM_SOCKET"
const SocketFileName = "humd.sock"

const RuntimeDirPerm = 0o700

func GlobalConfigDir() string {
	if dir := os.Getenv(EnvHome); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ProjectDirName
	}
	return filepath.Join(home, ProjectDirName)
}

func GlobalConfigFile() string {
	return filepath.Join(GlobalConfigDir(), ConfigFileName)
}

func ProjectConfigFile(startDir string) (string, bool) {
	dir, err := absolute(startDir)
	if err != nil {
		return "", false
	}
	global := absClean(GlobalConfigFile())
	for {
		candidate := filepath.Join(dir, ProjectDirName, ConfigFileName)
		if candidate != global {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

const GitDirName = ".git"

func ProjectRoot(startDir string) (string, error) {
	dir, err := absolute(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve project root from %q: %w", startDir, err)
	}
	if config, ok := ProjectConfigFile(dir); ok {
		dir = filepath.Dir(filepath.Dir(config))
	} else if root, ok := gitRoot(dir); ok {
		dir = root
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve project root %s: %w", dir, err)
	}
	return resolved, nil
}

func gitRoot(dir string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, GitDirName)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func SocketPath() string {
	if p := os.Getenv(EnvSocket); p != "" {
		return p
	}
	return filepath.Join(GlobalConfigDir(), SocketFileName)
}

func EnsureRuntimeDir() error {
	dir := filepath.Dir(SocketPath())
	if err := os.MkdirAll(dir, RuntimeDirPerm); err != nil {
		return fmt.Errorf("create runtime dir %s: %w", dir, err)
	}
	return nil
}

const LogFileName = "humd.error.log"

var executable = os.Executable

func LogFile() (string, bool) {
	exe, err := executable()
	if err != nil {
		return "", false
	}
	bin := filepath.Dir(exe)
	if filepath.Base(bin) != "bin" {
		return "", false
	}
	return filepath.Join(installPrefix(filepath.Dir(bin)), "var", "log", "hum", LogFileName), true
}

func installPrefix(dir string) string {
	segments := strings.Split(dir, string(filepath.Separator))
	switch {
	case len(segments) >= 4 && segments[len(segments)-3] == "Cellar":
		return strings.Join(segments[:len(segments)-3], string(filepath.Separator))
	case len(segments) >= 4 && segments[len(segments)-2] == "opt":
		return strings.Join(segments[:len(segments)-2], string(filepath.Separator))
	}
	return dir
}

var absolute = filepath.Abs

func absClean(p string) string {
	if abs, err := absolute(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}
