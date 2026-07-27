package paths

import (
	"fmt"
	"os"
	"path/filepath"
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

var absolute = filepath.Abs

func absClean(p string) string {
	if abs, err := absolute(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}
