package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

const (
	exitOK          = 0
	exitDaemonError = 1
	exitUsage       = 2
	exitUnreachable = 3
)

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

var control = map[string]protocol.Command{
	"ping":   protocol.CmdPing,
	"status": protocol.CmdStatus,
	"mute":   protocol.CmdMute,
	"stop":   protocol.CmdShutdown,
}

type options struct {
	asJSON  bool
	timeout time.Duration
}

func flagsFor(name string, opts *options, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.asJSON, "json", opts.asJSON, "print the daemon's raw response")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "how long to wait for the daemon")
	return flags
}

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

func operandsOf(name string, words []string, opts *options, stderr io.Writer) ([]string, bool) {
	var positional []string
	for {
		flags := flagsFor(name, opts, stderr)
		if err := flags.Parse(words); err != nil {
			return nil, false
		}
		if flags.NArg() == 0 {
			return positional, true
		}
		positional = append(positional, flags.Arg(0))
		words = flags.Args()[1:]
	}
}

func parse(words []string, opts *options, stderr io.Writer) (protocol.Request, int) {
	command, words := words[0], words[1:]

	if command == "theme" {
		return parseTheme(words, opts, stderr)
	}

	cmd, known := control[command]
	if !known {
		fmt.Fprintf(stderr, "hum: unknown command %q\n\n", command)
		fmt.Fprint(stderr, usage)
		return protocol.Request{}, exitUsage
	}

	operands, ok := operandsOf("hum "+command, words, opts, stderr)
	if !ok {
		return protocol.Request{}, exitUsage
	}
	if len(operands) != 0 {
		return unexpected(command, operands[0], stderr)
	}
	return protocol.Request{Command: cmd}, exitOK
}

func parseTheme(words []string, opts *options, stderr io.Writer) (protocol.Request, int) {
	operands, ok := operandsOf("hum theme", words, opts, stderr)
	if !ok {
		return protocol.Request{}, exitUsage
	}
	if len(operands) == 0 {
		fmt.Fprint(stderr, "hum theme: expected \"list\" or \"use <name>\"\n")
		return protocol.Request{}, exitUsage
	}

	switch operands[0] {
	case "list":
		if len(operands) > 1 {
			return unexpected("theme list", operands[1], stderr)
		}
		return protocol.Request{Command: protocol.CmdThemeList}, exitOK
	case "use":
		if len(operands) != 2 {
			fmt.Fprint(stderr, "hum theme use: expected exactly one theme name\n")
			return protocol.Request{}, exitUsage
		}
		return protocol.Request{Command: protocol.CmdThemeUse, Value: operands[1]}, exitOK
	default:
		fmt.Fprintf(stderr, "hum theme: unknown subcommand %q, expected \"list\" or \"use\"\n", operands[0])
		return protocol.Request{}, exitUsage
	}
}

func unexpected(command, operand string, stderr io.Writer) (protocol.Request, int) {
	fmt.Fprintf(stderr, "hum %s: unexpected argument %q\n", command, operand)
	return protocol.Request{}, exitUsage
}

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
