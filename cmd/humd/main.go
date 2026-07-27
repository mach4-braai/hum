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

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
