package renderer

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/theme"
)

const defaultSampleRate = 48000

type Options struct {
	SampleRate int
	Theme      theme.Theme
	Volume     float64
	Muted      bool
	Logger     *slog.Logger
}

type Renderer interface {
	Name() string
	Update(harmony.State) error
	Trigger(harmony.Phrase) error
	SetVolume(float64) error
	SetMuted(bool) error
	Close() error
}

type Themeable interface {
	SetTheme(theme.Theme) error
}

type Sampled interface {
	SampleRate() int
}

type constructor func(Options) (Renderer, error)

var (
	regMu    sync.Mutex
	registry = map[string]constructor{}
)

func Register(name string, ctor func(Options) (Renderer, error)) {
	if name == "" {
		panic("renderer: Register called with empty name")
	}
	if ctor == nil {
		panic("renderer: Register called with nil constructor for " + name)
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[name]; ok {
		panic("renderer: Register called twice for " + name)
	}
	registry[name] = ctor
}

func applyDefaults(opts Options) Options {
	if opts.SampleRate == 0 {
		opts.SampleRate = defaultSampleRate
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return opts
}

func Open(name string, opts Options) (Renderer, error) {
	regMu.Lock()
	ctor, ok := registry[name]
	regMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("renderer %q not registered; registered: %s", name, strings.Join(Names(), ", "))
	}
	return ctor(applyDefaults(opts))
}

func Names() []string {
	regMu.Lock()
	defer regMu.Unlock()
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
