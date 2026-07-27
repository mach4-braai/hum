// Package paths resolves the filesystem locations Hum uses for configuration
// and for its control socket. The daemon and the CLI must agree on these, so
// they are computed here rather than in either binary.
package paths

import (
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
