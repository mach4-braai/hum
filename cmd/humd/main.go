package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mach4-braai/hum/internal/audio"
	"github.com/mach4-braai/hum/internal/buildinfo"
	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
	"github.com/mach4-braai/hum/internal/transport"
)

const (
	exitOK          = 0
	exitError       = 1
	exitUsage       = 2
	exitInterrupted = 130
)

const (
	shutdownMargin  = 500 * time.Millisecond
	defaultRenderer = "audio"
	nopRendererName = "nop"
)

const usage = `usage: humd [flags]

Flags:
  --config <path>     global configuration file
  --socket <path>     socket to listen on
  --renderer <name>   output renderer
  --no-audio          use the silent renderer
  --log-level <level> debug, info, warn or error
  --version           print the version and exit
  --json              print the version as JSON
`

var (
	version        = buildinfo.UnknownVersion
	commit         = buildinfo.UnknownCommit
	date           = buildinfo.UnknownDate
	openRendererFn = openRenderer
)

func build() buildinfo.Info {
	return buildinfo.Resolve("humd", version, commit, date)
}

type options struct {
	configFile   string
	socket       string
	rendererName string
	noAudio      bool
	logLevel     string
	showVersion  bool
	asJSON       bool
}

func parseFlags(args []string, stderr io.Writer) (options, int) {
	var opts options
	flags := flag.NewFlagSet("humd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	flags.StringVar(&opts.configFile, "config", "", "global configuration file")
	flags.StringVar(&opts.socket, "socket", "", "socket to listen on")
	flags.StringVar(&opts.rendererName, "renderer", defaultRenderer, "output renderer")
	flags.BoolVar(&opts.noAudio, "no-audio", false, "use the silent renderer")
	flags.StringVar(&opts.logLevel, "log-level", "info", "debug, info, warn or error")
	flags.BoolVar(&opts.showVersion, "version", false, "print the version and exit")
	flags.BoolVar(&opts.asJSON, "json", false, "print the version as JSON")

	if err := flags.Parse(args); err != nil {
		return opts, exitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "humd: unexpected argument %q\n\n", flags.Arg(0))
		fmt.Fprint(stderr, usage)
		return opts, exitUsage
	}
	return opts, exitOK
}

func parseLevel(name string) (slog.Level, error) {
	switch name {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q, want debug, info, warn or error", name)
}

func openRenderer(name string, noAudio bool, opts renderer.Options, log *slog.Logger) (renderer.Renderer, error) {
	if noAudio {
		name = nopRendererName
	}
	r, err := renderer.Open(name, opts)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, audio.ErrNoDevice) {
		return nil, err
	}
	log.Warn("no audio device; continuing without sound", "renderer", name, "error", err)
	return renderer.Open(nopRendererName, opts)
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, code := parseFlags(args, stderr)
	if code != exitOK {
		return code
	}
	if opts.showVersion {
		if err := build().Write(stdout, opts.asJSON); err != nil {
			fmt.Fprintf(stderr, "humd: %v\n", err)
			return exitError
		}
		return exitOK
	}

	level, err := parseLevel(opts.logLevel)
	if err != nil {
		fmt.Fprintf(stderr, "humd: %v\n", err)
		return exitUsage
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	cfg, _, err := config.ResolveForSession(opts.configFile, "")
	if err != nil {
		log.Error("cannot resolve configuration", "error", err)
		return exitError
	}

	th, err := theme.Load(cfg.Music.Theme)
	if err != nil {
		log.Error("cannot load theme", "theme", cfg.Music.Theme, "error", err)
		return exitError
	}

	requested := opts.rendererName
	if opts.noAudio {
		requested = nopRendererName
	}
	render, err := openRendererFn(opts.rendererName, opts.noAudio, renderer.Options{
		Theme:  th,
		Volume: cfg.Audio.Volume,
		Muted:  cfg.Audio.Muted,
		Logger: log,
	}, log)
	if err != nil {
		log.Error("cannot open renderer", "renderer", opts.rendererName, "error", err)
		return exitError
	}

	d, err := newDaemon(log, cfg, th, render, requested, opts.configFile)
	if err != nil {
		log.Error("cannot start the daemon", "error", err)
		render.Close()
		return exitError
	}

	socket := opts.socket
	if socket == "" {
		socket = paths.SocketPath()
	}
	listener, err := transport.NewUnixListener(socket, transport.Options{Logger: log})
	if err != nil {
		log.Error("cannot listen", "socket", socket, "error", err)
		render.Close()
		return exitError
	}

	log.Info("humd listening",
		"socket", listener.Addr(),
		"renderer", render.Name(),
		"theme", th.Name,
		"root", cfg.Music.Root,
		"scale", cfg.Music.Scale,
		"version", build().Version,
	)

	return serve(d, listener, log, signals)
}

func serve(d *daemon, listener transport.Listener, log *slog.Logger, signals chan os.Signal) int {
	events, stopEvents := context.WithCancel(context.Background())
	defer stopEvents()
	go d.serveEvents(events)

	accept, stopAccepting := context.WithCancel(context.Background())
	defer stopAccepting()

	served := make(chan error, 1)
	go func() { served <- listener.Serve(accept, d.handle) }()

	select {
	case sig := <-signals:
		log.Info("shutting down", "signal", sig.String())
	case <-d.shutdown:
		log.Info("shutting down", "reason", "shutdown command")
	case err := <-served:
		if err != nil {
			log.Error("serve failed", "error", err)
			listener.Close()
			d.render.Close()
			return exitError
		}
		log.Info("shutting down", "reason", "the listener stopped serving")
		code := d.drain(signals, log)
		listener.Close()
		return code
	}

	stopAccepting()
	<-served

	code := d.drain(signals, log)
	listener.Close()
	return code
}

func (d *daemon) drain(signals <-chan os.Signal, log *slog.Logger) int {
	if err := d.render.Update(harmony.State{}); err != nil {
		log.Error("cannot release voices", "error", err)
	}

	log.Debug("waiting for voices to fade", "deadline", d.releaseWait)

	select {
	case <-time.After(d.releaseWait):
	case sig := <-signals:
		log.Warn("second signal, exiting immediately", "signal", sig.String())
		d.render.Close()
		return exitInterrupted
	}

	if err := d.render.Close(); err != nil {
		log.Error("cannot close the renderer", "error", err)
		return exitError
	}
	log.Info("stopped")
	return exitOK
}

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
