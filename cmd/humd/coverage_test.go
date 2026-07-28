package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/audio"
	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
	"github.com/mach4-braai/hum/internal/transport"
)

type bareRenderer struct{}

func (b *bareRenderer) Name() string                 { return "bare" }
func (b *bareRenderer) Update(harmony.State) error   { return nil }
func (b *bareRenderer) Trigger(harmony.Phrase) error { return nil }
func (b *bareRenderer) SetVolume(float64) error      { return nil }
func (b *bareRenderer) SetMuted(bool) error          { return nil }
func (b *bareRenderer) Close() error                 { return nil }

var _ renderer.Renderer = (*bareRenderer)(nil)

type failSetThemeRenderer struct{ recorder }

func (f *failSetThemeRenderer) SetTheme(theme.Theme) error { return errors.New("theme rejected") }

type closeErrRenderer struct{ recorder }

func (c *closeErrRenderer) Close() error { return errors.New("close failure") }

type updateErrRenderer struct{ recorder }

func (u *updateErrRenderer) Update(harmony.State) error { return errors.New("update failure") }

type triggerErrRenderer struct{ recorder }

func (t *triggerErrRenderer) Trigger(harmony.Phrase) error { return errors.New("trigger failure") }

type mutedErrRenderer struct{ recorder }

func (m *mutedErrRenderer) SetMuted(bool) error { return errors.New("mute failure") }

type volumeErrRenderer struct{ recorder }

func (v *volumeErrRenderer) SetVolume(float64) error { return errors.New("volume failure") }

type errListener struct {
	addr     string
	serveErr error
}

func (e *errListener) Serve(_ context.Context, _ transport.Handler) error { return e.serveErr }
func (e *errListener) Addr() string                                       { return e.addr }
func (e *errListener) Close() error                                       { return nil }

var _ transport.Listener = (*errListener)(nil)

var registerErrdevOnce sync.Once

func TestNewDaemonRejectsInvalidRoot(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Music.Root = "ZZZZZ"
	th, err := theme.Load("minimal")
	if err != nil {
		t.Fatalf("load theme: %v", err)
	}
	_, err = newDaemon(quietLogger(), &cfg, th, &recorder{}, "")
	if err == nil {
		t.Fatal("newDaemon with invalid root = nil, want an error")
	}
	if !strings.Contains(err.Error(), "music.root") {
		t.Errorf("error %q does not identify music.root", err)
	}
}

func TestNewDaemonRejectsInvalidScale(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Music.Scale = "klingon_pentatonic"
	th, err := theme.Load("minimal")
	if err != nil {
		t.Fatalf("load theme: %v", err)
	}
	_, err = newDaemon(quietLogger(), &cfg, th, &recorder{}, "")
	if err == nil {
		t.Fatal("newDaemon with invalid scale = nil, want an error")
	}
	if !strings.Contains(err.Error(), "music.scale") {
		t.Errorf("error %q does not identify music.scale", err)
	}
}

func TestHandleAfterDispatchStopsIsRefused(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)

	go func() {
		<-d.calls
		close(d.stopped)
	}()

	response := d.handle(protocol.Request{Command: protocol.CmdPing})
	if response.OK {
		t.Errorf("handle after stopped mid-flight = %+v, want failure", response)
	}
	if !strings.Contains(response.Error, "shutting down") {
		t.Errorf("error %q does not say the daemon is shutting down", response.Error)
	}
}

func TestDispatchRejectsInvalidRequest(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)

	resp := d.dispatch(protocol.Request{Command: protocol.Command("badcmd")})
	if resp.OK {
		t.Errorf("dispatch(invalid) = %+v, want failure", resp)
	}
	if resp.Error == "" {
		t.Error("dispatch(invalid) error is empty")
	}
}

func TestApplyEventSessionUpdatedRaisesUpdateCount(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "upd1", Workspace: "repo"}
	if resp := send(t, socket, protocol.Request{Event: &started})[0]; !resp.OK {
		t.Fatalf("session.started = %+v", resp)
	}

	updated := protocol.Event{Event: protocol.SessionUpdated, ID: "upd1", Title: "new title"}
	if resp := send(t, socket, protocol.Request{Event: &updated})[0]; !resp.OK {
		t.Fatalf("session.updated = %+v", resp)
	}

	status := statusOf(t, socket)
	if len(status.Sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(status.Sessions))
	}
	if status.Sessions[0].Updates != 1 {
		t.Errorf("Updates = %d, want 1 after session.updated", status.Sessions[0].Updates)
	}
}

func TestApplyEventSessionCancelledReleasesVoiceWithNoPhrase(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "can1"}
	if resp := send(t, socket, protocol.Request{Event: &started})[0]; !resp.OK {
		t.Fatalf("session.started = %+v", resp)
	}

	cancelled := protocol.Event{Event: protocol.SessionCancelled, ID: "can1"}
	if resp := send(t, socket, protocol.Request{Event: &cancelled})[0]; !resp.OK {
		t.Fatalf("session.cancelled = %+v", resp)
	}

	for _, call := range rec.history() {
		if strings.HasPrefix(call, "trigger/") {
			t.Errorf("session.cancelled emitted phrase %q, want none (cancelled_sounds=false in minimal)", call)
		}
	}

	if status := statusOf(t, socket); status.SoundingVoices != 0 {
		t.Errorf("sounding voices = %d after cancellation, want 0", status.SoundingVoices)
	}
}

func TestApplyEventTerminalForUnknownSessionIsRejected(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	ev := protocol.Event{Event: protocol.SessionCompleted, ID: "ghost99"}
	resp := send(t, socket, protocol.Request{Event: &ev})[0]
	if resp.OK {
		t.Errorf("session.completed for unknown id = %+v, want rejection", resp)
	}
}

func TestApplyEventTriggerErrorIsReportedButNotFatal(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &triggerErrRenderer{}
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "tr1"}
	if resp := send(t, socket, protocol.Request{Event: &started})[0]; !resp.OK {
		t.Fatalf("session.started = %+v", resp)
	}

	completed := protocol.Event{Event: protocol.SessionCompleted, ID: "tr1"}
	resp := send(t, socket, protocol.Request{Event: &completed})[0]
	if resp.OK {
		t.Errorf("session.completed with a failing trigger = %+v, want the failure reported to the client", resp)
	}

	status := statusOf(t, socket)
	if status.SoundingVoices != 0 {
		t.Errorf("sounding voices = %d after completion, want 0; the session must still advance", status.SoundingVoices)
	}
	if len(status.Sessions) != 1 || status.Sessions[0].State != "completed" {
		t.Errorf("status sessions = %+v, want tr1 recorded as completed", status.Sessions)
	}

	if resp := send(t, socket, protocol.Request{Command: protocol.CmdPing})[0]; !resp.OK {
		t.Errorf("ping after a renderer failure = %+v, want the daemon still serving", resp)
	}
}

func TestAdoptContextKeepsCurrentThemeWhenProjectThemeUnloadable(t *testing.T) {
	proj := project(t, "music:\n  theme: nosuchtheme\n")

	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	initialTheme := statusOf(t, socket).Theme

	resp := start(t, socket, "s1", proj)
	if !resp.OK {
		t.Fatalf("session.started with unloadable project theme = %+v, want ok (daemon keeps current theme)", resp)
	}

	if got := statusOf(t, socket).Theme; got != initialTheme {
		t.Errorf("theme = %q after unloadable project theme, want %q unchanged", got, initialTheme)
	}
}

func TestAdoptContextRejectsMalformedProjectConfig(t *testing.T) {
	proj := project(t, "music:\n  scale: notascale\n")

	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	resp := start(t, socket, "s1", proj)
	if resp.OK {
		t.Fatalf("session.started with invalid scale config = %+v, want rejection", resp)
	}

	if status := statusOf(t, socket); len(status.Sessions) != 0 {
		t.Errorf("status has %d sessions after rejected adoptContext, want 0", len(status.Sessions))
	}
}

func TestUseThemeWithNonThemeableRenderer(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &bareRenderer{}

	if err := d.useTheme("minimal"); err != nil {
		t.Errorf("useTheme with non-Themeable renderer = %v, want nil (SetTheme not called)", err)
	}
	if d.theme.Name != "minimal" {
		t.Errorf("theme name = %q after useTheme, want minimal", d.theme.Name)
	}
}

func TestUseThemeSetThemeErrorPropagates(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &failSetThemeRenderer{}

	err := d.useTheme("minimal")
	if err == nil {
		t.Fatal("useTheme with failing SetTheme = nil, want an error")
	}
	if !strings.Contains(err.Error(), "theme rejected") {
		t.Errorf("error %q does not contain the SetTheme failure message", err)
	}
}

func TestApplyCommandUnknownCommandReturnsError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)

	resp := d.applyCommand(protocol.Command("whatisthis"), "")
	if resp.OK {
		t.Errorf("applyCommand(unknown) = %+v, want failure", resp)
	}
	if !strings.Contains(resp.Error, "unknown command") {
		t.Errorf("error %q does not identify it as an unknown command", resp.Error)
	}
}

func TestApplyCommandVolumeAtBounds(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	for _, value := range []string{"0", "1"} {
		resp := send(t, socket, protocol.Request{Command: protocol.CmdVolume, Value: value})[0]
		if !resp.OK {
			t.Errorf("volume %s = %+v, want ok (inclusive bound)", value, resp)
		}
	}
}

func TestApplyCommandMuteRestoresOnUnmute(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	responses := send(t, socket,
		protocol.Request{Command: protocol.CmdMute},
		protocol.Request{Command: protocol.CmdUnmute},
	)
	for i, resp := range responses {
		if !resp.OK {
			t.Fatalf("response %d = %+v, want ok", i, resp)
		}
	}

	mutedCount := 0
	for _, call := range rec.history() {
		if call == "muted" {
			mutedCount++
		}
	}
	if mutedCount != 2 {
		t.Errorf("SetMuted called %d times, want 2 (once mute, once unmute)", mutedCount)
	}

	if got := statusOf(t, socket); got.Muted {
		t.Error("status.Muted = true after unmute, want false")
	}
}

func TestApplyCommandMuteErrorPropagates(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &mutedErrRenderer{}

	resp := d.applyCommand(protocol.CmdMute, "")
	if resp.OK {
		t.Errorf("applyCommand(mute) with failing SetMuted = %+v, want failure", resp)
	}
	if !strings.Contains(resp.Error, "mute failure") {
		t.Errorf("error %q does not contain the mute failure message", resp.Error)
	}
}

func TestApplyCommandVolumeErrorPropagates(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &volumeErrRenderer{}

	resp := d.applyCommand(protocol.CmdVolume, "0.5")
	if resp.OK {
		t.Errorf("applyCommand(volume) with failing SetVolume = %+v, want failure", resp)
	}
	if !strings.Contains(resp.Error, "volume failure") {
		t.Errorf("error %q does not contain the volume failure message", resp.Error)
	}
}

func TestApplyCommandThemeListPayloadShape(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	resp := send(t, socket, protocol.Request{Command: protocol.CmdThemeList})[0]
	if !resp.OK {
		t.Fatalf("theme.list = %+v, want ok", resp)
	}

	var data protocol.ThemeListPayload
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode theme.list payload: %v", err)
	}
	if len(data.Themes) == 0 {
		t.Error("theme.list returned empty themes slice, want at least one")
	}
	found := false
	for _, name := range data.Themes {
		if name == "minimal" {
			found = true
		}
	}
	if !found {
		t.Errorf("theme.list = %v, want 'minimal' listed", data.Themes)
	}
	if data.Active != "minimal" {
		t.Errorf("theme.list active = %q, want the daemon's current theme so a client can mark it", data.Active)
	}
}

func TestApplyCommandUnknownCommandViaRaw(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	resp := sendRaw(t, socket, `{"command":"totally_unknown_cmd"}`)
	if resp.OK {
		t.Errorf("unknown command via raw JSON = %+v, want failure", resp)
	}
}

func TestParseVolumeRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"notanumber", "1.5", "-0.1"} {
		if _, err := parseVolume(value); err == nil {
			t.Errorf("parseVolume(%q) = nil, want an error", value)
		}
	}
}

func TestApplyCommandVolumeInvalidValueDirectly(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)

	resp := d.applyCommand(protocol.CmdVolume, "notanumber")
	if resp.OK {
		t.Errorf("applyCommand(volume, notanumber) = %+v, want failure", resp)
	}
}

func TestPayloadMarshalError(t *testing.T) {
	resp := payload(math.NaN())
	if resp.OK {
		t.Errorf("payload(NaN) = %+v, want failure", resp)
	}
	if resp.Error == "" {
		t.Error("payload(NaN) error is empty, want a non-empty error message")
	}
}

func TestOpenRendererErrNoDeviceFallsBackToNop(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	registerErrdevOnce.Do(func() {
		renderer.Register("errdev", func(opts renderer.Options) (renderer.Renderer, error) {
			return nil, audio.ErrNoDevice
		})
	})

	r, err := openRenderer("errdev", false, renderer.Options{}, quietLogger())
	if err != nil {
		t.Fatalf("openRenderer with ErrNoDevice = %v, want fallback to nop", err)
	}
	defer r.Close()
	if r.Name() != "nop" {
		t.Errorf("renderer name = %q, want nop after ErrNoDevice fallback", r.Name())
	}
}

func TestServeListenerErrorExitsWithError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, rec := testDaemon(t)

	listener := &errListener{addr: "/tmp/err.sock", serveErr: errors.New("listener broke")}
	signals := make(chan os.Signal, 2)

	code := serve(d, listener, quietLogger(), signals)

	if code != exitError {
		t.Errorf("serve returned %d, want %d when listener errors immediately", code, exitError)
	}
	if rec.closeCount() != 1 {
		t.Errorf("renderer closed %d times, want exactly 1", rec.closeCount())
	}
}

func TestDrainCloseErrorExitsWithError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &closeErrRenderer{}
	d.releaseWait = time.Millisecond

	signals := make(chan os.Signal, 1)
	code := d.drain(signals, quietLogger())

	if code != exitError {
		t.Errorf("drain returned %d, want %d when Close errors", code, exitError)
	}
}

func TestDrainUpdateErrorStillCompletes(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d, _ := testDaemon(t)
	d.render = &updateErrRenderer{}
	d.releaseWait = time.Millisecond

	signals := make(chan os.Signal, 1)
	code := d.drain(signals, quietLogger())

	if code != exitOK {
		t.Errorf("drain returned %d, want %d (shutdown completes despite Update failure)", code, exitOK)
	}
}

func TestRunHappyPathViaShutdownCommand(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	socket := shortSocket(t)
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"--no-audio", "--socket", socket}, io.Discard, io.Discard)
	}()

	waitForSocket(t, socket)

	responses := send(t, socket, protocol.Request{Command: protocol.CmdShutdown})
	if !responses[0].OK {
		t.Fatalf("shutdown = %+v, want ok", responses[0])
	}

	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("run returned %d, want %d after shutdown command", code, exitOK)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not exit after shutdown command")
	}
}

func TestRunDefaultSocketPathFromEnv(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "humd.sock")
	t.Setenv("HUM_SOCKET", socket)

	done := make(chan int, 1)
	go func() {
		done <- run([]string{"--no-audio"}, io.Discard, io.Discard)
	}()

	waitForSocket(t, socket)

	responses := send(t, socket, protocol.Request{Command: protocol.CmdShutdown})
	if !responses[0].OK {
		t.Fatalf("shutdown = %+v, want ok", responses[0])
	}

	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("run (default socket) returned %d, want %d", code, exitOK)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not exit after shutdown command")
	}
}

func TestServeEventsReapsBehavior(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	send(t, socket,
		protocol.Request{Event: &protocol.Event{Event: protocol.SessionStarted, ID: "alive1"}},
		protocol.Request{Event: &protocol.Event{Event: protocol.SessionStarted, ID: "dead1"}},
		protocol.Request{Event: &protocol.Event{Event: protocol.SessionCompleted, ID: "dead1"}},
	)

	dropped := d.registry.Reap(0)
	if dropped != 1 {
		t.Errorf("Reap(0) dropped %d sessions, want 1 (the terminal one)", dropped)
	}

	status := statusOf(t, socket)
	for _, s := range status.Sessions {
		if s.ID == "dead1" {
			t.Errorf("terminal session dead1 still present after Reap, want it removed")
		}
	}

	found := false
	for _, s := range status.Sessions {
		if s.ID == "alive1" {
			found = true
		}
	}
	if !found {
		t.Error("active session alive1 was reaped, want it retained")
	}
}
