// Command hum is the client CLI for the Hum auditory display daemon.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

// Exit codes are part of the CLI contract: a CI script distinguishes "the work
// failed" from "Hum is not running", and the two must never collapse into 1.
const (
	exitOK          = 0
	exitDaemonError = 1
	exitUsage       = 2
	exitUnreachable = 3
)

// defaultTimeout bounds a control round trip. The daemon answers from memory,
// so a slow reply means it is wedged rather than busy.
const defaultTimeout = 2 * time.Second

const usage = `usage: hum [--json] [--timeout <duration>] <command> [flags]

Commands:
  init          write a project configuration
  start         announce a new work session
  stop          stop the daemon
  complete      mark a session completed
  fail          mark a session failed
  status        report daemon and session state
  mute          silence output without stopping
  doctor        diagnose the installation
  theme list    list available themes
  theme use     switch to a theme
  ping          check that the daemon is reachable
  help          print this message

Exit codes:
  0  success
  1  the daemon returned an error
  2  usage error
  3  the daemon is unreachable
`

// control maps each command that is a bare control request onto its protocol
// command. Commands carrying a payload or acting locally (init, start,
// complete, fail, doctor) arrive with their own issues rather than being
// guessed at here.
var control = map[string]protocol.Command{
	"ping":   protocol.CmdPing,
	"status": protocol.CmdStatus,
	"mute":   protocol.CmdMute,
	"stop":   protocol.CmdShutdown,
}

// options are accepted before or after the command, because both spellings are
// natural and rejecting one is a papercut rather than a contract.
type options struct {
	asJSON  bool
	timeout time.Duration
}

// flagsFor gives each command its own set over the same options, so a command
// can add its own flags later without another dispatcher.
func flagsFor(name string, opts *options, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.asJSON, "json", opts.asJSON, "print the daemon's raw response")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "how long to wait for the daemon")
	return flags
}

// run executes the CLI and returns the process exit code. It is separated from
// main so that exit codes and output streams are testable in process.
func run(args []string, stdout, stderr io.Writer) int {
	opts := options{timeout: defaultTimeout}
	global := flagsFor("hum", &opts, stderr)
	global.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := global.Parse(args); err != nil {
		return exitUsage
	}

	rest := global.Args()
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	request, code := parse(rest, &opts, stderr)
	if code != exitOK {
		return code
	}
	return send(request, opts.timeout, opts.asJSON, stdout, stderr)
}

// parse turns the command words into one request, or reports a usage error.
func parse(words []string, opts *options, stderr io.Writer) (protocol.Request, int) {
	command, operands := words[0], words[1:]

	if command == "theme" {
		return parseTheme(operands, opts, stderr)
	}

	cmd, known := control[command]
	if !known {
		fmt.Fprintf(stderr, "hum: unknown command %q\n\n", command)
		fmt.Fprint(stderr, usage)
		return protocol.Request{}, exitUsage
	}

	flags := flagsFor("hum "+command, opts, stderr)
	if err := flags.Parse(operands); err != nil {
		return protocol.Request{}, exitUsage
	}
	if flags.NArg() != 0 {
		return unexpected(command, flags.Arg(0), stderr)
	}
	return protocol.Request{Command: cmd}, exitOK
}

func parseTheme(operands []string, opts *options, stderr io.Writer) (protocol.Request, int) {
	flags := flagsFor("hum theme", opts, stderr)
	if err := flags.Parse(operands); err != nil {
		return protocol.Request{}, exitUsage
	}
	words := flags.Args()
	if len(words) == 0 {
		fmt.Fprint(stderr, "hum theme: expected \"list\" or \"use <name>\"\n")
		return protocol.Request{}, exitUsage
	}
	subcommand, rest := words[0], words[1:]

	switch subcommand {
	case "list":
		return finishTheme("theme list", protocol.Request{Command: protocol.CmdThemeList}, rest, opts, stderr)
	case "use":
		if len(rest) == 0 {
			fmt.Fprint(stderr, "hum theme use: expected exactly one theme name\n")
			return protocol.Request{}, exitUsage
		}
		request := protocol.Request{Command: protocol.CmdThemeUse, Value: rest[0]}
		return finishTheme("theme use", request, rest[1:], opts, stderr)
	default:
		fmt.Fprintf(stderr, "hum theme: unknown subcommand %q, expected \"list\" or \"use\"\n", subcommand)
		return protocol.Request{}, exitUsage
	}
}

// finishTheme parses whatever follows the subcommand's own operands. Go's flag
// package stops at the first positional, so trailing flags reach a set only
// once the positional has been taken off the front.
func finishTheme(name string, request protocol.Request, rest []string, opts *options, stderr io.Writer) (protocol.Request, int) {
	flags := flagsFor("hum "+name, opts, stderr)
	if err := flags.Parse(rest); err != nil {
		return protocol.Request{}, exitUsage
	}
	if flags.NArg() != 0 {
		return unexpected(name, flags.Arg(0), stderr)
	}
	return request, exitOK
}

// unexpected rejects trailing words rather than ignoring them, so a mistyped
// command is reported instead of silently doing something else.
func unexpected(command, operand string, stderr io.Writer) (protocol.Request, int) {
	fmt.Fprintf(stderr, "hum %s: unexpected argument %q\n", command, operand)
	return protocol.Request{}, exitUsage
}

// exit is a seam: os.Exit would end the test binary before it could observe
// that main forwards run's code.
var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
