package main

import "fmt"

func init() {
	register("stop", runStop)
}

func runStop(e *env, _ []string) int {
	fmt.Fprintln(e.stderr, "hum: to stop the daemon, use 'hum daemon stop'")
	return exitUsage
}
