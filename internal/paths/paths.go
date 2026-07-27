// Package paths resolves the filesystem locations Hum uses for configuration
// and for its control socket. The daemon and the CLI must agree on these, so
// they are computed here rather than in either binary.
package paths

import "os"

// EnvHome overrides the global configuration directory.
const EnvHome = "HUM_HOME"

// GlobalConfigDir returns the directory holding global configuration.
func GlobalConfigDir() string {
	return os.Getenv(EnvHome)
}
