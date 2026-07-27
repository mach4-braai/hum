// Command humd is the Hum daemon: it owns the session registry, the harmony
// engine and the audio renderer, and serves clients over a Unix socket.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `usage: humd [flags]`

// run executes the daemon and returns the process exit code. It is separated
// from main so that exit codes and output streams are testable in process.
func run(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, usage)
	return 2
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
