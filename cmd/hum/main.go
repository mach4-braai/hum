// Command hum is the client CLI for the Hum auditory display daemon.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `usage: hum <command> [flags]`

// run executes the CLI and returns the process exit code. It is separated from
// main so that exit codes and output streams are testable in process.
func run(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, usage)
	return 2
}

// exit is a seam: os.Exit would end the test binary before it could observe
// that main forwards run's code.
var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
