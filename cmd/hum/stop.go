package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

const stopDefaultTimeout = 10 * time.Second
const stopPollInterval = 5 * time.Millisecond

func init() {
	register("stop", runStop)
}

func runStop(e *env, words []string) int {
	rest, ok := operands(e, "stop", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "stop", rest[0])
	}
	if !e.opts.timeoutExplicit {
		e.opts.timeout = stopDefaultTimeout
	}
	deadline := time.Now().Add(e.opts.timeout)
	ans, err := query(protocol.Request{Command: protocol.CmdShutdown}, e.opts.timeout)
	if err != nil {
		if errors.Is(err, errNoDaemon) {
			fmt.Fprintln(e.stdout, "not running")
			return exitOK
		}
		return unreachable(e, err)
	}
	if e.opts.asJSON {
		printJSON(e, ans.raw)
	}
	if !ans.response.OK {
		return e.fail("hum: %s", responseError(ans.response))
	}
	return stopWaitForSocket(e, paths.SocketPath(), deadline)
}

func stopWaitForSocket(e *env, socket string, deadline time.Time) int {
	for {
		if _, err := os.Stat(socket); errors.Is(err, os.ErrNotExist) {
			return exitOK
		}
		if time.Now().After(deadline) {
			return e.fail("hum stop: the daemon at %s did not stop within %s", socket, e.opts.timeout)
		}
		time.Sleep(stopPollInterval)
	}
}
