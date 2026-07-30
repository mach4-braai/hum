# hum

Hum is a local-first daemon that renders work-session lifecycle events as an
ambient musical soundscape. One work session becomes one sustained drone in a
chosen key and scale; concurrent sessions form chords; completion and failure
each have a recognisable musical phrase.

It is an auditory display, not a notification system. Nothing beeps at you. The
point is to understand what your machine is doing without looking at it.

Hum knows nothing about any particular tool. Clients emit generic session events
over a Unix socket, so anything that can write a line of JSON can drive it.

## Install

### Homebrew

```sh
brew tap mach4-braai/tap
brew install hum
```

The formula builds from source; Go is the only build dependency on macOS. To
track the default branch instead of the latest release, use
`brew install --HEAD hum`.

Run the daemon under launchd or systemd:

```sh
brew services start hum
brew services stop hum
```

Logs go to `$(brew --prefix)/var/log/hum/humd.log` and `humd.error.log`. The
supervisor restarts `humd` only if it crashes, so a deliberate `hum stop` stays
stopped.

### From source

Requires [mise](https://mise.jdx.dev), which pins the Go toolchain. On Linux you
also need ALSA headers (`libasound2-dev` and `pkg-config`); macOS needs neither.

```sh
git clone https://github.com/mach4-braai/hum
cd hum
mise run build              # binaries into bin/
PREFIX=/usr/local mise run install
```

## Quickstart

Sixty seconds, from nothing to sound:

```sh
humd &                                      # start the daemon
hum start --id build --title "Build"        # a drone fades in
hum start --id tests --title "Test suite"   # a second voice joins it
hum status                                  # see what is sounding
hum stop                                    # fade out and exit
```

`hum status` reports the pitch each session was allocated, so you can correlate
what you hear with what is running:

```
ID     WORKSPACE  TITLE       STATE   NOTE  AGE
build             Build       active  D3    1s
tests             Test suite  active  F4    1s
```

Pass `--workspace <name>` to `hum start` to group sessions under a label.

## Commands

| Command | Description |
|---|---|
| `hum init` | write a project configuration file |
| `hum start` | announce a new work session |
| `hum stop` | fade out every voice and stop the daemon |
| `hum complete` | mark a session completed — its drone resolves and stops |
| `hum fail` | mark a session failed — a descending cadence, then the drone stops |
| `hum cancel` | mark a session abandoned — the drone stops without a cadence |
| `hum update` | report progress without ending the session; `--meta agents=N` widens the stereo image |
| `hum status` | report daemon state and every live session |
| `hum mute` / `hum unmute` | silence or resume output without stopping |
| `hum volume [fraction]` | report or set the output volume |
| `hum theme list` | list the available themes |
| `hum theme use <name>` | switch to a theme |
| `hum doctor` | diagnose the installation |
| `hum version` | print version, commit, date, Go version and platform |
| `hum ping` | check that the daemon is reachable |
| `humd` | the daemon itself |

See [integrations.md](docs/integrations.md) for wrappers that emit those events
directly.

Every command accepts `--json` for scripting and `--timeout` for how long to wait
on the daemon. Exit codes: `0` success, `1` the daemon returned an error, `2`
usage error, `3` the daemon is unreachable.

## Documentation

| Document | Contents |
|---|---|
| [configuration.md](docs/configuration.md) | file locations, precedence, every key and default, `HUM_HOME` and `HUM_SOCKET` |
| [protocol.md](docs/protocol.md) | the wire contract: framing, events, commands, responses |
| [integrations.md](docs/integrations.md) | shell wrapper, git hook and CI recipes |
| [cli.md](docs/cli.md) | the client's behaviour and output formats |
| [architecture.md](docs/architecture.md) | the rendering chain and the concurrency contract |
| [daemon.md](docs/daemon.md) | daemon wiring, state ownership and shutdown |
| [harmony.md](docs/harmony.md) | voice allocation, scales and phrases |
| [audio.md](docs/audio.md) | the mixer, oscillators and envelopes |
| [renderer.md](docs/renderer.md) | the renderer interface and registry |
| [themes.md](docs/themes.md) | the theme format and the built-in theme |
| [session.md](docs/session.md) | the session object and its state machine |
| [transport.md](docs/transport.md) | the Unix socket server and its lifecycle |
| [releasing.md](docs/releasing.md) | tagging a release, the Homebrew tap and the credential it needs |

Contributors should read [AGENTS.md](AGENTS.md) first: it carries the comment
policy, the coverage floor and the list of traps this codebase cannot express in
code.

## Platforms

macOS and Linux, `amd64` and `arm64`. The audio backend reaches AudioToolbox
through purego on macOS and needs no cgo; Linux links ALSA, so Linux builds
require a C toolchain until [#39](https://github.com/mach4-braai/hum/issues/39)
lands.

Windows archives are built for `amd64` and `arm64` and are unsupported: no CI job
executes them, and terminal width is not detected there, so `hum status` will not
truncate long titles. [#70](https://github.com/mach4-braai/hum/issues/70) is what
would change that.

## Licence

MIT. See [LICENSE](LICENSE).
