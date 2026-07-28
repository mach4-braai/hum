package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/session"
	"github.com/mach4-braai/hum/internal/theme"
)

const (
	defaultReapEvery  = time.Minute
	defaultReapAfter  = 5 * time.Minute
	audioTestDuration = 2 * time.Second
	audioTestGain     = 0.6
)

type call struct {
	request protocol.Request
	reply   chan protocol.Response
}

type daemon struct {
	log         *slog.Logger
	registry    *session.Registry
	engine      *harmony.Engine
	render      renderer.Renderer
	requested   string
	theme       theme.Theme
	globalFile  string
	releaseWait time.Duration
	reapEvery   time.Duration
	reapAfter   time.Duration

	contextOwner string
	volume       float64
	muted        bool

	calls    chan call
	stopped  chan struct{}
	shutdown chan struct{}
}

func tuning(cfg *config.Config) (harmony.Pitch, harmony.Scale, error) {
	class, err := harmony.ParseNoteClass(cfg.Music.Root)
	if err != nil {
		return harmony.Pitch{}, harmony.Scale{}, fmt.Errorf("music.root: %w", err)
	}
	scale, err := harmony.LookupScale(cfg.Music.Scale)
	if err != nil {
		return harmony.Pitch{}, harmony.Scale{}, fmt.Errorf("music.scale: %w", err)
	}
	return harmony.Pitch{Class: class, Octave: droneOctave}, scale, nil
}

func releaseWaitFor(th theme.Theme) time.Duration {
	return time.Duration(th.Drone.Release*float64(time.Second)) + shutdownMargin
}

func newDaemon(log *slog.Logger, cfg *config.Config, th theme.Theme, r renderer.Renderer, requested, globalFile string) (*daemon, error) {
	root, scale, err := tuning(cfg)
	if err != nil {
		return nil, err
	}

	return &daemon{
		log:         log,
		registry:    session.New(),
		engine:      harmony.NewEngine(root, scale, th.PhraseSpec()),
		render:      r,
		requested:   requested,
		theme:       th,
		globalFile:  globalFile,
		releaseWait: releaseWaitFor(th),
		reapEvery:   defaultReapEvery,
		reapAfter:   defaultReapAfter,
		volume:      cfg.Audio.Volume,
		muted:       cfg.Audio.Muted,
		calls:       make(chan call),
		stopped:     make(chan struct{}),
		shutdown:    make(chan struct{}),
	}, nil
}

func (d *daemon) serveEvents(ctx context.Context) {
	defer close(d.stopped)

	reap := time.NewTicker(d.reapEvery)
	defer reap.Stop()

	for {
		select {
		case c := <-d.calls:
			c.reply <- d.dispatch(c.request)
		case <-reap.C:
			if dropped := d.registry.Reap(d.reapAfter); dropped > 0 {
				d.log.Debug("reaped terminal sessions", "count", dropped)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *daemon) handle(request protocol.Request) protocol.Response {
	reply := make(chan protocol.Response, 1)
	select {
	case d.calls <- call{request: request, reply: reply}:
	case <-d.stopped:
		return protocol.Response{OK: false, Error: "daemon is shutting down"}
	}
	select {
	case response := <-reply:
		return response
	case <-d.stopped:
		return protocol.Response{OK: false, Error: "daemon is shutting down"}
	}
}

func (d *daemon) dispatch(request protocol.Request) protocol.Response {
	if err := request.Validate(); err != nil {
		return failure(err)
	}
	if request.Event != nil {
		return d.applyEvent(*request.Event)
	}
	return d.applyCommand(request.Command, request.Value)
}

func (d *daemon) applyEvent(event protocol.Event) protocol.Response {
	if event.Event == protocol.SessionStarted {
		if err := d.adoptContext(event.Root); err != nil {
			return failure(err)
		}
	}

	change, err := d.registry.Apply(event)
	if err != nil {
		return failure(err)
	}

	state, phrases := d.engine.Apply(change)

	var rendered error
	if err := d.render.Update(state); err != nil {
		d.log.Error("renderer update failed", "error", err, "session", event.ID)
		rendered = err
	}
	for _, phrase := range phrases {
		if err := d.render.Trigger(phrase); err != nil {
			d.log.Error("renderer trigger failed", "error", err, "phrase", string(phrase.Kind))
			if rendered == nil {
				rendered = err
			}
		}
	}

	d.log.Info("session event",
		"event", string(event.Event),
		"session", event.ID,
		"state", string(change.Session.State),
		"voices", len(state.Voices),
	)
	if rendered != nil {
		return failure(fmt.Errorf("session tracked, but the renderer failed: %w", rendered))
	}
	return protocol.Response{OK: true}
}

func (d *daemon) adoptContext(root string) error {
	cfg, _, err := config.ResolveForSession(d.globalFile, root)
	if err != nil {
		return err
	}
	if d.sounding() {
		return nil
	}

	tune, scale, err := tuning(cfg)
	if err != nil {
		return err
	}
	if err := d.engine.Retune(tune, scale); err != nil {
		return err
	}

	owner := root
	if root != "" {
		if canonical, err := config.CanonicalRoot(root); err == nil {
			owner = canonical
		}
	}

	if cfg.Music.Theme != d.theme.Name {
		if err := d.useTheme(cfg.Music.Theme); err != nil {
			d.log.Warn("keeping the current theme", "requested", cfg.Music.Theme, "error", err)
		}
	}
	d.contextOwner = owner
	d.log.Info("adopted musical context", "root", cfg.Music.Root, "scale", cfg.Music.Scale, "theme", d.theme.Name, "project", owner)
	return nil
}

func (d *daemon) sounding() bool {
	for _, s := range d.registry.Snapshot() {
		if !s.State.Terminal() {
			return true
		}
	}
	return false
}

func (d *daemon) useTheme(name string) error {
	th, err := theme.Load(name)
	if err != nil {
		return err
	}
	if setter, ok := d.render.(renderer.Themeable); ok {
		if err := setter.SetTheme(th); err != nil {
			return err
		}
	}
	d.theme = th
	d.releaseWait = releaseWaitFor(th)
	d.engine.SetPhraseSpec(th.PhraseSpec())
	return nil
}

func (d *daemon) applyCommand(command protocol.Command, value string) protocol.Response {
	switch command {
	case protocol.CmdPing:
		return protocol.Response{OK: true}

	case protocol.CmdStatus:
		return d.status()

	case protocol.CmdMute, protocol.CmdUnmute:
		muted := command == protocol.CmdMute
		if err := d.render.SetMuted(muted); err != nil {
			return failure(err)
		}
		d.muted = muted
		return protocol.Response{OK: true}

	case protocol.CmdVolume:
		volume, err := parseVolume(value)
		if err != nil {
			return failure(err)
		}
		if err := d.render.SetVolume(volume); err != nil {
			return failure(err)
		}
		d.volume = volume
		return protocol.Response{OK: true}

	case protocol.CmdThemeList:
		return payload(protocol.ThemeListPayload{Themes: theme.List(), Active: d.theme.Name})

	case protocol.CmdThemeUse:
		if err := d.useTheme(value); err != nil {
			return failure(err)
		}
		return payload(protocol.ThemeUsePayload{Theme: d.theme.Name})

	case protocol.CmdAudioTest:
		return d.audioTest()

	case protocol.CmdShutdown:
		d.requestShutdown()
		return protocol.Response{OK: true}
	}
	return failure(fmt.Errorf("%w: %q", protocol.ErrUnknownCommand, command))
}

func (d *daemon) requestShutdown() {
	select {
	case <-d.shutdown:
	default:
		close(d.shutdown)
	}
}

func (d *daemon) audioTest() protocol.Response {
	root, _ := d.engine.Tuning()
	phrase := harmony.Phrase{
		Kind: harmony.PhraseTest,
		Notes: []harmony.Note{{
			Pitch:    root,
			Duration: audioTestDuration,
			Gain:     audioTestGain,
		}},
	}
	if err := d.render.Trigger(phrase); err != nil {
		return failure(err)
	}
	name := d.render.Name()
	return payload(protocol.AudioTestPayload{
		Played:   name != nopRendererName && !d.muted,
		Renderer: name,
		Muted:    d.muted,
		Seconds:  audioTestDuration.Seconds(),
	})
}

func (d *daemon) sampleRate() int {
	if sampled, ok := d.render.(renderer.Sampled); ok {
		return sampled.SampleRate()
	}
	return 0
}

func (d *daemon) status() protocol.Response {
	snapshot := d.registry.Snapshot()
	state := d.engine.State()

	pitches := make(map[string]string, len(state.Voices))
	for _, v := range state.Voices {
		pitches[v.SessionID] = v.Pitch.String()
	}

	sessions := make([]protocol.SessionPayload, len(snapshot))
	for i, s := range snapshot {
		sessions[i] = protocol.SessionPayload{
			ID:        s.ID,
			Workspace: s.Workspace,
			Title:     s.Title,
			State:     string(s.State),
			Pitch:     pitches[s.ID],
			Priority:  s.Priority,
			Metadata:  s.Metadata,
			Updates:   s.Updates,
			Seconds:   s.Duration().Seconds(),
		}
	}

	root, scale := d.engine.Tuning()
	return payload(protocol.StatusPayload{
		Sessions:          sessions,
		Theme:             d.theme.Name,
		Root:              root.String(),
		Scale:             scale.Name,
		ContextOwner:      d.contextOwner,
		Renderer:          d.render.Name(),
		RendererRequested: d.requested,
		SampleRate:        d.sampleRate(),
		Version:           version,
		Volume:            d.volume,
		Muted:             d.muted,
		SoundingVoices:    len(state.Voices),
	})
}

func failure(err error) protocol.Response {
	return protocol.Response{OK: false, Error: err.Error()}
}

func payload(v any) protocol.Response {
	data, err := json.Marshal(v)
	if err != nil {
		return failure(err)
	}
	return protocol.Response{OK: true, Data: data}
}

func parseVolume(value string) (float64, error) {
	request := protocol.Request{Command: protocol.CmdVolume, Value: value}
	if err := request.Validate(); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(value, 64)
}
