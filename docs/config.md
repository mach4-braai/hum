# Configuration

## Overview

`internal/config` loads, merges, and validates the hum configuration. It exposes three operations:

- `Default()` — in-memory defaults, no I/O.
- `Load(path)` — decode one YAML file into `Config`, or return `(nil, nil)` if absent.
- `Resolve(cliOverrides, startDir)` — fold all four layers into a final `Config` with provenance.

## Four Layers and Precedence

Resolution applies layers in ascending precedence order, so later layers win:

```
Defaults → Global → Project → CLI
```

| Layer     | Source                                | `Layer` constant   |
|-----------|---------------------------------------|--------------------|
| `default` | Hard-coded in `Default()`             | `LayerDefault`     |
| `global`  | `~/.hum/config.yaml` (or `$HUM_HOME`) | `LayerGlobal`      |
| `project` | Nearest `.hum/config.yaml` upward     | `LayerProject`     |
| `cli`     | `cliOverrides` map passed by the CLI  | `LayerCLI`         |

`Resolve` finds the global path via `paths.GlobalConfigFile()` and the project path via `paths.ProjectConfigFile(startDir)`, which walks up the directory tree and skips the global path if encountered.

## Provenance

`Provenance` is `map[string]Layer`. Every field path in the resolved config maps to the layer that supplied it. Fields untouched by any layer report `LayerDefault`.

Field paths tracked: `project.name`, `music.root`, `music.scale`, `music.theme`, `audio.volume`, `audio.muted`.

`hum doctor` uses provenance to explain why a given field has its current value.

## Pointer-Field Technique and Why `volume: 0` Forces It

A naive approach decodes each YAML file into a `Config` directly, then falls back to the previous layer for zero-valued fields. This conflates "field absent from the file" with "field present and explicitly set to its zero value". A project file containing `audio: volume: 0` would be indistinguishable from a project file that omits `audio` entirely, so the global `volume: 0.6` would silently survive — the wrong answer.

The fix is an internal `layerData` struct with pointer fields:

```go
type layerAudio struct {
    Volume *float64 `yaml:"volume"`
    Muted  *bool    `yaml:"muted"`
}
```

`yaml.v3` leaves a pointer `nil` when the corresponding key is absent from the YAML, and allocates a pointer to the decoded value when the key is present (including `volume: 0`). `applyLayer` checks each pointer for nil before overwriting the accumulator, so a present-zero beats a previous-layer non-zero.

`reflect` is not used; the field set is small and explicit nil checks are faster, clearer, and safer under refactoring.

## Field Table

| Field path     | Default               | YAML key                     | CLI override key |
|----------------|-----------------------|------------------------------|------------------|
| `project.name` | `""` (empty)          | `project.name`               | `project.name`   |
| `music.root`   | `"D"`                 | `music.root`                 | `music.root`     |
| `music.scale`  | `"minor_pentatonic"`  | `music.scale`                | `music.scale`    |
| `music.theme`  | `"minimal"`           | `music.theme`                | `music.theme`    |
| `audio.volume` | `0.6`                 | `audio.volume`               | `audio.volume`   |
| `audio.muted`  | `false`               | `audio.muted`                | `audio.muted`    |

`minimal` is the default theme because PRD.md §20 permits exactly one built-in theme for the MVP. `orchestra` from the §13 example is Phase 2.

Root and scale names are not validated against the harmony tables here. That cross-package validation is the lead's wiring step in wave 2. `Validate` here checks only non-emptiness of `music.root`, `music.scale`, `music.theme`, and the volume range.

## `KnownFields(true)` Decision

Both `Load` and `loadLayer` use `yaml.Decoder` with `KnownFields(true)`. This causes yaml.v3 to reject any key not present in the target struct, turning a typo such as a top-level `theme: orchestra` (instead of `music.theme: orchestra`) into an immediate error naming the offending field and line number, rather than silently producing a config that ignores the user's intent.

## NaN Guard

`Validate` and the `audio.volume` CLI parser both use:

```go
if !(v >= 0 && v <= 1) { … }
```

not the superficially equivalent:

```go
if v < 0 || v > 1 { … }
```

Every comparison against `NaN` is `false` in IEEE 754. The naive form evaluates `NaN < 0` (false) and `NaN > 1` (false), so `false || false = false`, and NaN slips through as valid. The negated form evaluates `!(NaN >= 0 && NaN <= 1)` = `!(false && false)` = `!false = true`, correctly rejecting NaN.

The same guard applies to `+Inf` and `-Inf`:
- `+Inf >= 0` is `true`, but `+Inf <= 1` is `false`, so the guard catches it.
- `-Inf >= 0` is `false`, so the guard catches it immediately.

`strconv.ParseFloat` returns NaN and Inf without error, making this guard essential for CLI-supplied volume strings.

## Musical validation

`Validate` checks `music.root` with `harmony.ParseNoteClass` and `music.scale`
with `harmony.LookupScale`, so a typo is rejected when the config is resolved
rather than at the moment a session tries to sound. `music.root` is a bare note
class: `D` and `F#` resolve, `D2` does not, because the octave is the engine's
choice and not the user's.

`music.theme` is checked for emptiness only. Themes are user-extensible files
under `$HUM_HOME/themes/`, so the set of valid names is not known to this
package; `internal/theme` reports an unknown theme when it fails to load one.

The dependency runs one way: `internal/config` imports `internal/harmony`, never
the reverse.
