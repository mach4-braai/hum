package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

const stopDefaultTimeout = 10 * time.Second
const stopPollInterval = 5 * time.Millisecond

func init() {
	register("daemon", runDaemon)
}

func supervisorRestartCommand(supervisor string) string {
	if supervisor == "launchd" {
		return "brew services restart hum"
	}
	return "systemctl --user restart humd"
}

func runDaemon(e *env, words []string) int {
	fs := flagsFor("hum daemon", e.opts, e.stderr, nil)
	if err := fs.Parse(words); err != nil {
		return exitUsage
	}
	noteExplicit(fs, e.opts)
	rest := fs.Args()
	if len(rest) == 0 {
		return e.usagef("hum daemon: expected \"stop\"")
	}
	switch rest[0] {
	case "stop":
		return daemonStop(e, rest[1:])
	default:
		return e.usagef("hum daemon: unknown subcommand %q; want \"stop\"", rest[0])
	}
}

func daemonStop(e *env, words []string) int {
	var force bool
	rest, ok := operands(e, "daemon stop", words, func(fs *flag.FlagSet) {
		fs.BoolVar(&force, "force", force, "")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "daemon stop", rest[0])
	}
	if !e.opts.timeoutExplicit {
		e.opts.timeout = stopDefaultTimeout
	}

	supervisor, supervised := detectSupervisor()
	if supervised && !force {
		return daemonStopSupervised(e, supervisor)
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
	code := stopWaitForSocket(e, paths.SocketPath(), deadline)
	if supervised && code == exitOK {
		fmt.Fprintln(e.stdout, supervisorRestartCommand(supervisor))
	}
	return code
}

func probeDaemon(timeout time.Duration) error {
	socket := paths.SocketPath()
	conn, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return classifyDialError(err, socket)
	}
	conn.Close()
	return nil
}

func daemonStopSupervised(e *env, supervisor string) int {
	err := probeDaemon(e.opts.timeout)
	if errors.Is(err, errNoDaemon) {
		fmt.Fprintln(e.stdout, "not running")
		return exitOK
	}
	if err != nil {
		return unreachable(e, err)
	}
	fmt.Fprintln(e.stderr, supervisorRestartCommand(supervisor))
	return exitUsage
}

func stopWaitForSocket(e *env, socket string, deadline time.Time) int {
	for {
		if _, err := os.Stat(socket); errors.Is(err, os.ErrNotExist) {
			return exitOK
		}
		if time.Now().After(deadline) {
			return e.fail("hum daemon stop: the daemon at %s did not stop within %s", socket, e.opts.timeout)
		}
		time.Sleep(stopPollInterval)
	}
}
