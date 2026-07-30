# Configuration

Hum reads YAML configuration from up to two files and accepts overrides
from the CLI. This document covers where those files live, how values
compose, every recognised key, and the internal design decisions that
make the system behave correctly.

## File locations

| File | Path |
|---|---|
| Global | `~/.hum/config.yaml`, or `$HUM_HOME/config.yaml` if `HUM_HOME` is set |
| Project | Nearest `.hum/config.yaml` at or above the working directory |

`paths.GlobalConfigFile()` derives the global path from `GlobalConfigDir()`,
which returns `$HUM_HOME` when set, otherwise `~/.hum`.

`paths.ProjectConfigFile(startDir)` walks upward from `startDir` stopping at
the first `.hum/config.yaml` it finds. It skips the path if it resolves to
the global file, so a `~/.hum/config.yaml` encountered mid-walk is never
double-counted as a project config.

Both files are optional. A missing file is not an error; `Load` returns
`(nil, nil)`.

## Environment variables

| Variable | Overrides | Default | Notes |
|---|---|---|---|
| `HUM_HOME` | Config directory path | `~/.hum` | Changes both the global config location and the default socket path |
| `HUM_SOCKET` | Unix socket path | `$HUM_HOME/humd.sock` | Must fit a Unix socket address |

**`HUM_SOCKET` length trap.** Unix socket addresses are null-terminated
inside a fixed `sun_path` buffer: 104 bytes on macOS, 108 on Linux. A path
that fits on one platform may be rejected on the other with `ENAMETOOLONG`
or `bind: invalid argument`. If you set `HUM_SOCKET` to an absolute path
under a deep directory, count the bytes. The default `$HUM_HOME/humd.sock`
fits safely from a typical home directory, but a deeply nested `$HUM_HOME`
can still exceed the limit. `hum doctor` checks this.

## Precedence

Resolution folds four layers into a single `Config`. The direction matters:
**later layers win**. Reading from lowest to highest priority:

```
Defaults → Global → Project → CLI
```

Equivalently, reading from highest to lowest priority (the PRD §12 order):

```
CLI → Project → Global → Defaults
```

Both descriptions are correct and describe the same system. A reader who
mixes them up gets the answer exactly backwards: a CLI flag does not lose
to a project file; it wins over it.

| Layer | Source | `Layer` constant |
|---|---|---|
| `default` | Hard-coded in `Default()` | `LayerDefault` |
| `global` | `~/.hum/config.yaml` (or `$HUM_HOME`) | `LayerGlobal` |
| `project` | Nearest `.hum/config.yaml` upward | `LayerProject` |
| `cli` | `cliOverrides` map passed by the CLI | `LayerCLI` |

`Resolve` finds the global path via `paths.GlobalConfigFile()` and the
project path via `paths.ProjectConfigFile(startDir)`.

### Provenance

`Provenance` is `map[string]Layer`. Every field path in the resolved config
maps to the layer that supplied it. Fields untouched by any layer report
`LayerDefault`. `hum doctor` uses provenance to explain which layer set a
given field.

Field paths tracked: `project.name`, `music.root`, `music.octave`,
`music.scale`, `music.theme`, `audio.volume`, `audio.muted`.

## Keys and defaults

| Field path | Default | YAML key | CLI override key |
|---|---|---|---|
| `project.name` | `""` (empty) | `project.name` | `project.name` |
| `music.root` | `"D"` | `music.root` | `music.root` |
| `music.octave` | `3` | `music.octave` | `music.octave` |
| `music.scale` | `"minor_pentatonic"` | `music.scale` | `music.scale` |
| `music.theme` | `"minimal"` | `music.theme` | `music.theme` |
| `audio.volume` | `0.6` | `audio.volume` | `audio.volume` |
| `audio.muted` | `false` | `audio.muted` | `audio.muted` |

`minimal` is the default theme because PRD.md §20 permits exactly one
built-in theme for the MVP. `orchestra` from the §13 example is Phase 2.

`music.root` is a bare note class: `D` and `F#` resolve; `D2` does not. The
register lives in `music.octave` instead, so the class and the register can be
set from different layers — a project may pick the note while the global file
decides how low the machine plays it.

`music.octave` is the octave the drone root sounds in, in scientific pitch
notation: `3` puts a root of D at D3, 146.8 Hz. Harmonies sound above it, up to
two octaves higher, so the audible span at octave 3 is D3 to D5. Valid range is
`[1, 6]` (`config.MinOctave`, `config.MaxOctave`): octave 1 keeps the
fundamental above about 30 Hz, and 6 is the highest octave whose two-octave
harmony ceiling still lands inside MIDI 127 for every note class.

The default is 3 rather than 2 on register grounds, not physiology. Huron
reports F2–G5 as the region of maximum pitch weight, the earcon literature
recommends a floor around 125–150 Hz for tones meant to carry information, and
small laptop drivers roll off below roughly 150–200 Hz — a 73 Hz fundamental is
felt more than heard, and what reaches the ear is largely the second harmonic
the drone adds at `2f`. Pitch weight varies continuously, so this is a taste
default, not a threshold: set `octave: 2` for a deeper bed and accept the
roughness that comes with it.

`music.scale` and `music.theme` are validated on resolution. Root and scale
are checked against the harmony tables (`harmony.ParseNoteClass`,
`harmony.LookupScale`). Theme is checked for emptiness only — themes are
user-extensible files under `$HUM_HOME/themes/`, so the full set of valid
names is not known to this package; `internal/theme` reports an unknown
theme when it fails to load one.

## Worked example

A complete `config.yaml` showing every key at once:

```yaml
project:
  name: my-project

music:
  root: D
  octave: 3
  scale: minor_pentatonic
  theme: minimal

audio:
  volume: 0.6
  muted: false
```

All keys are optional. An empty file is valid; defaults fill every omitted
field. `hum init` writes a generated version of this file with the valid
scale and theme names listed as comments.

## Writing settings back

Three commands persist changes through the config layer:

- `hum volume N` and `hum mute` / `hum unmute` write `audio.volume` and
  `audio.muted` to the **global** config after the daemon accepts the
  change. The daemon is asked first and the file second: the reverse order
  would leave a config claiming `muted: true` while sound kept playing.
- `hum theme use NAME` writes `music.theme` to the global config the same
  way.
- `hum init` writes a fresh `.hum/config.yaml` (or the global file with
  `--global`) via `config.Write`.

Both operations:

- `Patch(path, values)` — set named field paths in an existing file,
  leaving everything else alone. Used by the three persisting commands
  above.
- `Write(path, data)` — replace the whole file. Used by `hum init`.

Both create the parent directory (`0700`), write a temporary file beside
the target, `fsync` it, `chmod` it to `0600`, and `rename` it into place.
A partial write would corrupt the user's entire configuration, and `rename`
within one directory is the only atomic replacement POSIX offers.

`Patch` validates every value *before* opening the file, so a rejected
`audio.volume` of `1.5` leaves the previous file intact rather than
truncating it and then failing. An unrecognised field path is
`ErrUnknownKey`.

## Per-project musical context

The daemon holds **one** musical context at a time. When a `session.started`
event carries a project root, the daemon adopts that project's config
(root, scale, theme) **only when no session is currently sounding**. While
anything is sounding, the established context persists and a session joining
from a different project inherits it rather than retuning it. Two concurrent
tonal centres would sound like two unrelated pieces of music playing at
once.

`hum status` reports `context_owner`: the project whose configuration
supplied the current root, scale and theme. It is omitted when none did.
Which project won is never a mystery.

Roots are canonicalised with `filepath.EvalSymlinks`, so a symlinked path
and its target are one project rather than two.

---

## Contributor reference

The sections below cover the internal design of `internal/config`. They
explain the traps the implementation avoids and why each technique was
chosen.

### Pointer-field technique and why `volume: 0` forces it

A naive approach decodes each YAML file into a `Config` directly, then
falls back to the previous layer for zero-valued fields. This conflates
"field absent from the file" with "field present and explicitly set to its
zero value". A project file containing `audio: volume: 0` would be
indistinguishable from a project file that omits `audio` entirely, so the
global `volume: 0.6` would silently survive — the wrong answer.

The fix is an internal `layerData` struct with pointer fields:

```go
type layerAudio struct {
    Volume *float64 `yaml:"volume"`
    Muted  *bool    `yaml:"muted"`
}
```

`yaml.v3` leaves a pointer `nil` when the corresponding key is absent from
the YAML, and allocates a pointer to the decoded value when the key is
present (including `volume: 0`). `applyLayer` checks each pointer for nil
before overwriting the accumulator, so a present-zero beats a previous-layer
non-zero.

`reflect` is not used; the field set is small and explicit nil checks are
faster, clearer, and safer under refactoring.

### `KnownFields(true)` decision

Both `Load` and `loadLayer` use `yaml.Decoder` with `KnownFields(true)`.
This causes yaml.v3 to reject any key not present in the target struct,
turning a typo such as a top-level `theme: orchestra` (instead of
`music.theme: orchestra`) into an immediate error naming the offending field
and line number, rather than silently producing a config that ignores the
user's intent.

### NaN guard

`Validate` and the `audio.volume` CLI parser both use:

```go
if !(v >= 0 && v <= 1) { … }
```

not the superficially equivalent:

```go
if v < 0 || v > 1 { … }
```

Every comparison against `NaN` is `false` in IEEE 754. The naive form
evaluates `NaN < 0` (false) and `NaN > 1` (false), so `false || false =
false`, and NaN slips through as valid. The negated form evaluates
`!(NaN >= 0 && NaN <= 1)` = `!(false && false)` = `!false = true`,
correctly rejecting NaN.

The same guard applies to `+Inf` and `-Inf`:
- `+Inf >= 0` is `true`, but `+Inf <= 1` is `false`, so the guard catches it.
- `-Inf >= 0` is `false`, so the guard catches it immediately.

`strconv.ParseFloat` returns NaN and Inf without error, making this guard
essential for CLI-supplied volume strings.

### Musical validation

`Validate` checks `music.root` with `harmony.ParseNoteClass` and
`music.scale` with `harmony.LookupScale`, so a typo is rejected when the
config is resolved rather than at the moment a session tries to sound.

`music.theme` is checked for emptiness only. The dependency runs one way:
`internal/config` imports `internal/harmony`, never the reverse.

### Why `Patch` decodes into `yaml.Node`

Decoding into `Config` and re-encoding would silently delete anything the
struct does not model — comments, key order, and any key a newer version of
hum writes. `hum init` emits a commented file listing the valid scales and
themes, so a `hum volume 0.4` that stripped those comments would degrade the
file every time it was touched.

`yaml.Node` keeps comments, order and unmodelled keys. `Patch` walks the
mapping to the requested field, creating intermediate mappings when they are
missing, replaces the value node in place, and carries the old node's
comments across so a trailing `# the current choice` survives a value
change. A key that exists but is not a mapping where one is needed is
replaced, because the alternative is refusing to fix a file the user has
already broken.

### Finding the project root

`paths.ProjectRoot(startDir)` answers "which project is this?" for the
client, which is the only process that knows: under `brew services` the
daemon's working directory is `$HOME`. Precedence, first match wins:

1. The directory owning the nearest `.hum/config.yaml` at or above
   `startDir`.
2. The nearest directory at or above it containing `.git` (a file, as in a
   worktree, counts).
3. `startDir` itself.

The result is passed through `filepath.EvalSymlinks`, so a session started
through a symlinked path lands in the same musical context as one started
through the canonical path instead of being counted as a second project.
