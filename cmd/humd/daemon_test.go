package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
	"github.com/mach4-braai/hum/internal/transport"
)

type recorder struct {
	mu      sync.Mutex
	calls   []string
	updates []harmony.State
	closes  int
}

var (
	_ renderer.Renderer  = (*recorder)(nil)
	_ renderer.Themeable = (*recorder)(nil)
)

func (r *recorder) Name() string { return "recorder" }

func (r *recorder) Update(s harmony.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "update")
	r.updates = append(r.updates, s)
	return nil
}

func (r *recorder) Trigger(p harmony.Phrase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "trigger/"+string(p.Kind))
	return nil
}

func (r *recorder) SetVolume(v float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "volume")
	return nil
}

func (r *recorder) SetMuted(m bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "muted")
	return nil
}

func (r *recorder) SetTheme(t theme.Theme) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "theme/"+t.Name)
	return nil
}

func (r *recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "close")
	r.closes++
	return nil
}

func (r *recorder) history() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recorder) recordedUpdates() []harmony.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]harmony.State(nil), r.updates...)
}

func (r *recorder) closeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closes
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDaemon(t *testing.T) (*daemon, *recorder) {
	t.Helper()
	t.Setenv("HUM_HOME", t.TempDir())

	cfg, _, err := config.ResolveForSession("", "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	th, err := theme.Load(cfg.Music.Theme)
	if err != nil {
		t.Fatalf("load theme: %v", err)
	}

	rec := &recorder{}
	d, err := newDaemon(quietLogger(), cfg, th, rec, "")
	if err != nil {
		t.Fatalf("newDaemon: %v", err)
	}
	d.releaseWait = 10 * time.Millisecond
	return d, rec
}

func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "humd.sock")
}

func startDaemon(t *testing.T, d *daemon) (string, chan os.Signal, chan int) {
	t.Helper()

	socket := shortSocket(t)
	listener, err := transport.NewUnixListener(socket, transport.Options{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	signals := make(chan os.Signal, 2)
	done := make(chan int, 1)
	go func() { done <- serve(d, listener, quietLogger(), signals) }()

	waitForSocket(t, socket)
	return socket, signals, done
}

func waitForSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("socket %s never accepted a connection", socket)
}

func send(t *testing.T, socket string, requests ...protocol.Request) []protocol.Response {
	t.Helper()

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("dial %s: %v", socket, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)
	responses := make([]protocol.Response, 0, len(requests))
	for _, request := range requests {
		if err := encoder.Encode(request); err != nil {
			t.Fatalf("encode %+v: %v", request, err)
		}
		var response protocol.Response
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("decode response to %+v: %v", request, err)
		}
		responses = append(responses, response)
	}
	return responses
}

func event(kind protocol.EventType, id string) protocol.Request {
	e := protocol.Event{Event: kind, ID: id}
	return protocol.Request{Event: &e}
}

func TestSessionLifecycleReachesTheRenderer(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)

	started := protocol.Event{Event: protocol.SessionStarted, ID: "123", Title: "Validate PR #142", Workspace: "tofu"}
	responses := send(t, socket,
		protocol.Request{Event: &started},
		event(protocol.SessionCompleted, "123"),
	)

	for i, response := range responses {
		if !response.OK {
			t.Fatalf("response %d = %+v, want ok", i, response)
		}
	}

	updates := rec.recordedUpdates()
	if len(updates) != 2 {
		t.Fatalf("recorded %d updates, want 2", len(updates))
	}
	if len(updates[0].Voices) != 1 {
		t.Errorf("first update carried %d voices, want 1", len(updates[0].Voices))
	}
	if len(updates[1].Voices) != 0 {
		t.Errorf("second update carried %d voices, want 0", len(updates[1].Voices))
	}

	history := rec.history()
	want := []string{"update", "update", "trigger/completion"}
	if strings.Join(history, ",") != strings.Join(want, ",") {
		t.Errorf("call history = %v, want %v", history, want)
	}

	signals <- syscall.SIGTERM
	if code := <-done; code != exitOK {
		t.Errorf("serve returned %d, want %d", code, exitOK)
	}
}

func TestStatusReportsTheActiveSession(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "s1", Title: "Validate PR #142", Workspace: "tofu"}
	if responses := send(t, socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v", responses[0])
	}

	responses := send(t, socket, protocol.Request{Command: protocol.CmdStatus})
	if !responses[0].OK {
		t.Fatalf("status = %+v", responses[0])
	}

	var status statusPayload
	if err := json.Unmarshal(responses[0].Data, &status); err != nil {
		t.Fatalf("decode status payload: %v", err)
	}
	if len(status.Sessions) != 1 {
		t.Fatalf("status reported %d sessions, want 1", len(status.Sessions))
	}
	got := status.Sessions[0]
	if got.ID != "s1" || got.Title != "Validate PR #142" || got.Workspace != "tofu" || got.State != "active" {
		t.Errorf("status session = %+v, want id s1, title \"Validate PR #142\", workspace tofu, state active", got)
	}
	if status.SoundingVoice != 1 {
		t.Errorf("sounding voices = %d, want 1", status.SoundingVoice)
	}
	if status.Renderer != "recorder" {
		t.Errorf("renderer = %q, want recorder", status.Renderer)
	}
}

func TestControlCommandsProxyToTheRenderer(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	responses := send(t, socket,
		protocol.Request{Command: protocol.CmdPing},
		protocol.Request{Command: protocol.CmdMute},
		protocol.Request{Command: protocol.CmdUnmute},
		protocol.Request{Command: protocol.CmdVolume, Value: "0.25"},
		protocol.Request{Command: protocol.CmdThemeList},
		protocol.Request{Command: protocol.CmdThemeUse, Value: "minimal"},
	)
	for i, response := range responses {
		if !response.OK {
			t.Fatalf("response %d = %+v, want ok", i, response)
		}
	}

	var themes map[string][]string
	if err := json.Unmarshal(responses[4].Data, &themes); err != nil {
		t.Fatalf("decode theme list: %v", err)
	}
	if len(themes["themes"]) == 0 || themes["themes"][0] != "minimal" {
		t.Errorf("theme list = %v, want minimal listed", themes["themes"])
	}

	history := strings.Join(rec.history(), ",")
	for _, want := range []string{"muted", "volume", "theme/minimal"} {
		if !strings.Contains(history, want) {
			t.Errorf("call history %q is missing %q", history, want)
		}
	}
}

func TestUnknownThemeIsRejectedWithoutChangingTheCurrentOne(t *testing.T) {
	d, _ := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	responses := send(t, socket, protocol.Request{Command: protocol.CmdThemeUse, Value: "orchestra"})
	if responses[0].OK {
		t.Fatalf("theme.use orchestra = %+v, want a failure", responses[0])
	}
	if !strings.Contains(responses[0].Error, "orchestra") {
		t.Errorf("error %q does not name the requested theme", responses[0].Error)
	}

	responses = send(t, socket, protocol.Request{Command: protocol.CmdStatus})
	var status statusPayload
	if err := json.Unmarshal(responses[0].Data, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Theme != "minimal" {
		t.Errorf("theme = %q after a failed switch, want minimal", status.Theme)
	}
}

func TestARendererFailureDoesNotLoseTheSession(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &failingRenderer{}
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	started := protocol.Event{Event: protocol.SessionStarted, ID: "keep", Title: "still tracked"}
	if responses := send(t, socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v, want ok despite the renderer failing", responses[0])
	}

	responses := send(t, socket, protocol.Request{Command: protocol.CmdStatus})
	var status statusPayload
	if err := json.Unmarshal(responses[0].Data, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(status.Sessions) != 1 || status.Sessions[0].ID != "keep" {
		t.Errorf("status sessions = %+v, want the session still tracked", status.Sessions)
	}
}

func TestShutdownCommandStopsTheDaemon(t *testing.T) {
	d, rec := testDaemon(t)
	socket, _, done := startDaemon(t, d)

	if responses := send(t, socket, protocol.Request{Command: protocol.CmdShutdown}); !responses[0].OK {
		t.Fatalf("shutdown = %+v, want ok", responses[0])
	}

	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("serve returned %d, want %d", code, exitOK)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown command did not stop the daemon")
	}

	if rec.closeCount() != 1 {
		t.Errorf("renderer closed %d times, want exactly 1", rec.closeCount())
	}
}

func TestGracefulShutdownReleasesVoicesBeforeClosing(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)

	started := protocol.Event{Event: protocol.SessionStarted, ID: "sounding", Title: "long work"}
	if responses := send(t, socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v", responses[0])
	}

	signals <- syscall.SIGTERM
	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("serve returned %d, want %d", code, exitOK)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not stop the daemon")
	}

	if rec.closeCount() != 1 {
		t.Errorf("renderer closed %d times, want exactly 1", rec.closeCount())
	}

	history := rec.history()
	if len(history) < 2 {
		t.Fatalf("call history = %v, want a release then a close", history)
	}
	if history[len(history)-1] != "close" {
		t.Fatalf("last call = %q, want close", history[len(history)-1])
	}
	if history[len(history)-2] != "update" {
		t.Errorf("call before close = %q, want the update that releases every drone", history[len(history)-2])
	}

	updates := rec.recordedUpdates()
	if final := updates[len(updates)-1]; len(final.Voices) != 0 {
		t.Errorf("final update carried %d voices, want 0 so every drone is released", len(final.Voices))
	}

	if _, err := os.Stat(socket); err == nil {
		t.Errorf("socket %s still exists after a clean shutdown", socket)
	}
}

func TestSecondSignalShortCircuitsTheFade(t *testing.T) {
	d, rec := testDaemon(t)
	d.releaseWait = 30 * time.Second
	socket, signals, done := startDaemon(t, d)

	started := protocol.Event{Event: protocol.SessionStarted, ID: "sounding"}
	if responses := send(t, socket, protocol.Request{Event: &started}); !responses[0].OK {
		t.Fatalf("session.started = %+v", responses[0])
	}

	signals <- syscall.SIGTERM
	signals <- syscall.SIGTERM

	select {
	case code := <-done:
		if code != exitInterrupted {
			t.Errorf("serve returned %d, want %d after a second signal", code, exitInterrupted)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a second signal did not short-circuit the fade deadline")
	}

	if rec.closeCount() != 1 {
		t.Errorf("renderer closed %d times, want exactly 1", rec.closeCount())
	}
	if _, err := os.Stat(socket); err == nil {
		t.Errorf("socket %s still exists after an interrupted shutdown", socket)
	}
}

func TestRequestsAfterTheEventLoopStopsAreRefused(t *testing.T) {
	d, _ := testDaemon(t)
	close(d.stopped)

	response := d.handle(protocol.Request{Command: protocol.CmdPing})
	if response.OK {
		t.Errorf("handle after the loop stopped = %+v, want a failure", response)
	}
	if !strings.Contains(response.Error, "shutting down") {
		t.Errorf("error %q does not say the daemon is shutting down", response.Error)
	}
}

func sendRaw(t *testing.T, socket, line string) protocol.Response {
	t.Helper()

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("dial %s: %v", socket, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}

	var response protocol.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("decode response to %q: %v", line, err)
	}
	return response
}

func TestInvalidRequestIsRejectedWithoutTouchingState(t *testing.T) {
	d, rec := testDaemon(t)
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	for _, line := range []string{
		`{"command":"volume","value":"NaN"}`,
		`{"command":"volume","value":"1.7"}`,
		`{"command":"nonsense"}`,
		`{"event":"session.exploded","id":"x"}`,
		`{"event":"session.started","id":"x","root":"relative/path"}`,
	} {
		if response := sendRaw(t, socket, line); response.OK {
			t.Errorf("%s = %+v, want a rejection", line, response)
		}
	}

	for _, call := range rec.history() {
		if call == "volume" {
			t.Error("a rejected volume reached the renderer")
		}
		if call == "update" {
			t.Error("a rejected event reached the renderer")
		}
	}
}

type failingRenderer struct{ recorder }

func (f *failingRenderer) Update(harmony.State) error { return errRendererBroken }

func (f *failingRenderer) Trigger(harmony.Phrase) error { return errRendererBroken }

var errRendererBroken = errors.New("renderer is broken")
