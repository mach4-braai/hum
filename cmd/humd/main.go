// Command humd is the Hum auditory display daemon: session registry, harmony
// engine, and audio renderer served over a Unix socket.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `usage: humd [flags]`

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
