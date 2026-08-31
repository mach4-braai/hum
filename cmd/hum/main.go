package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mach4-braai/hum/internal/buildinfo"
)

const (
	exitOK          = 0
	exitDaemonError = 1
	exitUsage       = 2
	exitUnreachable = 3
)

const defaultTimeout = 2 * time.Second

var (
	version = buildinfo.UnknownVersion
	commit  = buildinfo.UnknownCommit
	date    = buildinfo.UnknownDate
)

const usage = `usage: hum [--json] [--timeout <duration>] <command> [flags]

Commands:
  init          write a project configuration
  start         announce a new work session
  daemon stop   stop the daemon
  complete      mark a session completed
  fail          mark a session failed — the work failed, not hum
  cancel        mark a session abandoned without running to an end
  update        report progress on a session without ending it
  status        report daemon and session state
  mute          silence output without stopping
  unmute        resume output
  volume        report or set the output volume
  doctor        diagnose the installation
  theme list    list available themes
  theme use     switch to a theme
  ping          check that the daemon is reachable
  version       print the version
  help          print this message

Exit codes:
  0  success
  1  the daemon returned an error
  2  usage error
  3  the daemon is unreachable
`

type options struct {
	asJSON          bool
	timeout         time.Duration
	timeoutExplicit bool
}

type env struct {
	stdout io.Writer
	stderr io.Writer
	opts   *options
}

func (e *env) usagef(format string, a ...any) int {
	fmt.Fprintf(e.stderr, format+"\n", a...)
	return exitUsage
}

func (e *env) fail(format string, a ...any) int {
	fmt.Fprintf(e.stderr, format+"\n", a...)
	return exitDaemonError
}

var commands = map[string]func(*env, []string) int{}

func register(name string, run func(*env, []string) int) {
	if _, taken := commands[name]; taken {
		panic("hum: command registered twice: " + name)
	}
	commands[name] = run
}

func flagsFor(name string, opts *options, stderr io.Writer, bind func(*flag.FlagSet)) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.asJSON, "json", opts.asJSON, "print the daemon's raw response")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "how long to wait for the daemon")
	if bind != nil {
		bind(flags)
	}
	return flags
}

func noteExplicit(flags *flag.FlagSet, opts *options) {
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "timeout" {
			opts.timeoutExplicit = true
		}
	})
}

func operandsOf(name string, words []string, opts *options, stderr io.Writer, bind func(*flag.FlagSet)) ([]string, bool) {
	var positional []string
	for {
		flags := flagsFor(name, opts, stderr, bind)
		if err := flags.Parse(words); err != nil {
			return nil, false
		}
		noteExplicit(flags, opts)
		if flags.NArg() == 0 {
			return positional, true
		}
		positional = append(positional, flags.Arg(0))
		words = flags.Args()[1:]
	}
}

func operands(e *env, command string, words []string, bind func(*flag.FlagSet)) ([]string, bool) {
	return operandsOf("hum "+command, words, e.opts, e.stderr, bind)
}

func unexpected(e *env, command, operand string) int {
	return e.usagef("hum %s: unexpected argument %q", command, operand)
}

func run(args []string, stdout, stderr io.Writer) int {
	opts := options{timeout: defaultTimeout}
	global := flagsFor("hum", &opts, stderr, nil)
	global.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := global.Parse(args); err != nil {
		return exitUsage
	}
	noteExplicit(global, &opts)

	rest := global.Args()
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	command, known := commands[rest[0]]
	if !known {
		fmt.Fprintf(stderr, "hum: unknown command %q\n\n", rest[0])
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	return command(&env{stdout: stdout, stderr: stderr, opts: &opts}, rest[1:])
}

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
