// Package paths resolves where Hum keeps its configuration and control socket.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvHome overrides the global configuration directory.
const EnvHome = "HUM_HOME"

// ConfigFileName is the configuration filename at both global and project level.
const ConfigFileName = "config.yaml"

// ProjectDirName is the per-project configuration directory.
const ProjectDirName = ".hum"

// EnvSocket overrides the control socket path.
const EnvSocket = "HUM_SOCKET"

// SocketFileName lives inside the config dir so one HUM_HOME moves all state.
const SocketFileName = "humd.sock"

// RuntimeDirPerm keeps the socket private to its owner.
const RuntimeDirPerm = 0o700

// GlobalConfigDir returns $HUM_HOME when set, otherwise ~/.hum.
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

// GlobalConfigFile returns the path to the global configuration file.
func GlobalConfigFile() string {
	return filepath.Join(GlobalConfigDir(), ConfigFileName)
}

// ProjectConfigFile walks upward from startDir, as git locates its root.
// Skips the global config file, which a client under $HOME would otherwise match.
func ProjectConfigFile(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
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

// SocketPath returns $HUM_SOCKET when set, otherwise humd.sock in the config dir.
// An override is returned verbatim; callers validate rather than normalise it.
func SocketPath() string {
	if p := os.Getenv(EnvSocket); p != "" {
		return p
	}
	return filepath.Join(GlobalConfigDir(), SocketFileName)
}

// EnsureRuntimeDir creates the socket's directory, idempotently. It sets
// RuntimeDirPerm only on directories it creates; the parent may be shared.
func EnsureRuntimeDir() error {
	dir := filepath.Dir(SocketPath())
	if err := os.MkdirAll(dir, RuntimeDirPerm); err != nil {
		return fmt.Errorf("create runtime dir %s: %w", dir, err)
	}
	return nil
}

// absClean resolves p against the working directory for like-form comparison.
func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}
