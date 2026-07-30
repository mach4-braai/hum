package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
)

const (
	defaultSummaryEvery = 5 * time.Minute
	defaultLogWindow    = time.Minute
	maxThrottleKeys     = 64
)

type throttle struct {
	window     time.Duration
	now        func() time.Time
	last       map[string]time.Time
	missed     map[string]int
	suppressed int
}

func newThrottle(window time.Duration) *throttle {
	return &throttle{
		window: window,
		now:    time.Now,
		last:   make(map[string]time.Time),
		missed: make(map[string]int),
	}
}

func (t *throttle) admit(key string) (int, bool) {
	if len(t.last) >= maxThrottleKeys {
		t.last = make(map[string]time.Time)
		t.missed = make(map[string]int)
	}
	now := t.now()
	if last, seen := t.last[key]; seen && now.Sub(last) < t.window {
		t.missed[key]++
		t.suppressed++
		return 0, false
	}
	held := t.missed[key]
	delete(t.missed, key)
	t.last[key] = now
	return held, true
}

func (t *throttle) drainSuppressed() int {
	n := t.suppressed
	t.suppressed = 0
	return n
}

func (d *daemon) throttled(level slog.Level, msg string, err error, key, value string) {
	held, admit := d.throttle.admit(msg + "\x00" + err.Error())
	if !admit {
		return
	}
	args := []any{"error", err, key, value}
	if held > 0 {
		args = append(args, "repeats", held)
	}
	d.log.Log(context.Background(), level, msg, args...)
}

func (d *daemon) rendererFailed(msg string, err error, key, value string) {
	d.throttled(slog.LevelError, msg, err, key, value)
}

func (d *daemon) logEvent(event protocol.Event, state string, voices int) {
	if event.Event == protocol.SessionUpdated {
		d.log.Debug("session event", "event", string(event.Event), "session", event.ID, "state", state, "voices", voices)
		return
	}
	d.log.Info("session event", "event", string(event.Event), "session", event.ID, "state", state, "voices", voices)
}

func (d *daemon) logContext(changed bool, root, scale, owner string) {
	level := slog.LevelDebug
	if changed {
		level = slog.LevelInfo
	}
	d.log.Log(context.Background(), level, "adopted musical context",
		"root", root, "scale", scale, "theme", d.theme.Name, "project", owner)
}

func (d *daemon) droppedPhrases() int {
	if dropper, ok := d.render.(renderer.PhraseDropper); ok {
		return dropper.DroppedPhrases()
	}
	return 0
}

func (d *daemon) logSummary() {
	events, reaped := d.events, d.reaped
	d.events, d.reaped = 0, 0
	suppressed := d.throttle.drainSuppressed()

	active := 0
	for _, s := range d.registry.Snapshot() {
		if !s.State.Terminal() {
			active++
		}
	}
	voices := len(d.engine.State().Voices)

	if active == 0 && voices == 0 && events == 0 && reaped == 0 && suppressed == 0 {
		return
	}
	d.log.Info("soundscape",
		"sessions", active,
		"voices", voices,
		"events", events,
		"reaped", reaped,
		"dropped_phrases", d.droppedPhrases(),
		"suppressed", suppressed,
	)
}
