# Renderer Package

`internal/renderer` defines the seam between the daemon's event loop and any audio or output backend. The daemon calls `Update` and `Trigger`; the backend implements `Renderer`.

## Interface

```go
type Renderer interface {
    Name() string
    Update(harmony.State) error   // sustained voices; idempotent
    Trigger(harmony.Phrase) error // one-shot; must not block
    SetVolume(float64) error
    SetMuted(bool) error
    Close() error                 // safe to call twice
}
```

## Concurrency Contract

`Update` is called only from the daemon's single event goroutine. It must be idempotent because the daemon calls it on every state change, including no-op transitions. A renderer that hands state off to an audio callback thread is responsible for its own internal locking; the interface makes no guarantee about which thread reads the state.

`Trigger` must not block the caller. The event goroutine must remain responsive while a phrase plays out over time. A blocking `Trigger` implementation would stall every subsequent `Update` for the duration of the phrase.

`Close` must be safe to call twice. The daemon calls it on graceful shutdown; a second call from a deferred cleanup must not panic or corrupt state. Implementations MUST be idempotent on double close.

## Registry

`Register(name, ctor)` is called from `init` functions, matching the `database/sql` driver convention. Duplicate registration panics at startup: a silently shadowed renderer would produce wrong audio with no error at runtime, making the failure undebuggable. An empty name or nil constructor also panic for the same reason — both represent a broken caller.

`Open(name, opts)` errors on an unknown name and includes the list of registered names in the message, so a misconfigured daemon tells you exactly what is available.

`Names()` returns a sorted copy. Callers may mutate the returned slice without corrupting the registry.

## Options and Defaults

```go
type Options struct {
    SampleRate int
    Theme      theme.Theme
    Volume     float64
    Muted      bool
    Logger     *slog.Logger
}
```

`Open` applies defaults before passing `Options` to the constructor:

| Field        | Zero value | Default                                    |
|--------------|------------|--------------------------------------------|
| `SampleRate` | 0          | 48000                                      |
| `Logger`     | nil        | `slog.Default()`                           |
| `Volume`     | 0          | `Theme.Drone.Gain` if theme is set, else 0.6 |

A theme is considered set when `Theme.Name` is non-empty. Volume 0 is treated as unset because the daemon always sets a positive initial volume from config; a renderer playing silence at init would be confusing.

## NopRenderer

`NopRenderer` ships in the production package, not as a test helper, because the daemon itself uses it when `--no-audio` is passed. Tests that verify daemon behaviour (session lifecycle, graceful shutdown, volume commands) run against `NopRenderer` whether or not an audio device is present. Moving it to a `_test.go` file would require either duplicating it or forcing production code to use the real audio renderer in CI.

`NopRenderer` is concurrency-safe: `Update` and `Trigger` may be called from any goroutine while accessor methods (`Updates`, `Triggers`, `Volume`, `Muted`, `Closes`) are read from test goroutines.

Accessor methods return deep copies of recorded slices. A test that mutates the returned slice cannot silently corrupt the next assertion.

`SetVolume` rejects values outside `[0, 1]` using the form `!(v >= 0 && v <= 1)`. The equivalent `v < 0 || v > 1` accepts `NaN` because every comparison against `NaN` is false. The daemon proxies the `hum volume` command directly to `SetVolume`; accepting `NaN` would silently corrupt the volume state.

## Audio Import Prohibition

`internal/renderer` does not import `internal/audio`. The renderer interface is the seam that makes `internal/audio` pluggable. If `renderer` imported `audio`, registering the audio renderer would become a build-time dependency instead of a runtime one, defeating the seam and preventing `NopRenderer`-only builds. The import absence is verified with `grep -r "internal/audio" internal/renderer`, which returns no results.
