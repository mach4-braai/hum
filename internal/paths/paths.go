// Package paths resolves where Hum keeps its configuration and control socket.
// The daemon and CLI must agree on these, so they are computed once here.
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

// SocketFileName is the daemon's control socket, kept inside the global config
// directory so one HUM_HOME relocates all of Hum's state.
const SocketFileName = "humd.sock"

// RuntimeDirPerm keeps the socket private: anything able to open it can drive the
// user's audio output.
const RuntimeDirPerm = 0o700

// GlobalConfigDir returns $HUM_HOME when set, otherwise ~/.hum. An undeterminable
// home yields a relative ".hum" so a cosmetic path problem cannot stop startup.
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

// ProjectConfigFile walks upward from startDir for a project configuration file,
// as git locates its root, and reports whether one was found.
//
// The global config file is skipped despite matching the shape searched for: it
// lives at ~/.hum/config.yaml, so a client under the home directory would apply
// it as project config and occupy two layers of the precedence chain.
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

// SocketPath returns $HUM_SOCKET when set, otherwise humd.sock inside the global
// config directory. The override exists because a deep home can push the default
// past sun_path. An override is returned verbatim: resolving a relative one here
// would yield a different absolute path per process, so callers validate instead.
func SocketPath() string {
	if p := os.Getenv(EnvSocket); p != "" {
		return p
	}
	return filepath.Join(GlobalConfigDir(), SocketFileName)
}

// EnsureRuntimeDir creates the directory holding the control socket. It is
// idempotent, and applies RuntimeDirPerm only to directories it creates: the
// parent may be shared, such as /tmp under a HUM_SOCKET override.
func EnsureRuntimeDir() error {
	dir := filepath.Dir(SocketPath())
	if err := os.MkdirAll(dir, RuntimeDirPerm); err != nil {
		return fmt.Errorf("create runtime dir %s: %w", dir, err)
	}
	return nil
}

// absClean resolves p against the working directory so path comparisons are made
// between like forms.
func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}
