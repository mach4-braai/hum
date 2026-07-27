// Package paths resolves the filesystem locations Hum uses for configuration
// and for its control socket. The daemon and the CLI must agree on these, so
// they are computed here rather than in either binary.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvHome overrides the global configuration directory.
const EnvHome = "HUM_HOME"

// GlobalConfigDir returns the directory holding global configuration: $HUM_HOME
// when set, otherwise ~/.hum. A home directory that cannot be determined yields
// a bare ".hum" relative to the working directory, which keeps the caller
// working rather than failing at startup over a cosmetic path.
func GlobalConfigDir() string {
	if dir := os.Getenv(EnvHome); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".hum"
	}
	return filepath.Join(home, ".hum")
}

// ConfigFileName is the configuration filename used at both the global and the
// project level, per PRD.md section 12.
const ConfigFileName = "config.yaml"

// GlobalConfigFile returns the path to the global configuration file.
func GlobalConfigFile() string {
	return filepath.Join(GlobalConfigDir(), ConfigFileName)
}

// ProjectDirName is the per-project configuration directory, per PRD.md
// section 12.
const ProjectDirName = ".hum"

// ProjectConfigFile walks upward from startDir looking for a project
// configuration file, returning the first match and whether one was found.
// Clients run from anywhere inside a project, so discovery mirrors the way git
// locates its root.
//
// The global configuration file is skipped even though it matches the shape
// being searched for. Global config lives at ~/.hum/config.yaml, so a client
// run anywhere under the home directory would otherwise discover it and apply
// it as project config, letting one file occupy two layers of PRD.md
// section 12's precedence chain at the wrong priority.
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

// absClean resolves p against the working directory, falling back to a cleaned
// relative path when that is not possible. Used so path comparisons are made
// between like forms.
func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// EnvSocket overrides the control socket path.
const EnvSocket = "HUM_SOCKET"

// SocketFileName is the daemon's control socket, kept inside the global config
// directory so a single HUM_HOME relocates all of Hum's state.
const SocketFileName = "humd.sock"

// SocketPath returns the path of the daemon's control socket: $HUM_SOCKET when
// set, otherwise humd.sock inside the global config directory. HUM_SOCKET
// exists because a deep home directory can push the default past the platform's
// sun_path limit, which is 104 bytes on macOS and 108 on Linux.
func SocketPath() string {
	if p := os.Getenv(EnvSocket); p != "" {
		return p
	}
	return filepath.Join(GlobalConfigDir(), SocketFileName)
}

// RuntimeDirPerm keeps the control socket private to its owner. Anything able to
// open the socket can drive the user's audio output.
const RuntimeDirPerm = 0o700

// EnsureRuntimeDir creates the directory that holds the control socket. It is
// idempotent, because every daemon start after the first finds it present.
func EnsureRuntimeDir() error {
	dir := filepath.Dir(SocketPath())
	if err := os.MkdirAll(dir, RuntimeDirPerm); err != nil {
		return fmt.Errorf("create runtime dir %s: %w", dir, err)
	}
	return nil
}
