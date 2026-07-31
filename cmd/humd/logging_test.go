package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/protocol"
)

func captureLog(d *daemon, level slog.Level) *bytes.Buffer {
	var buf bytes.Buffer
	d.log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level}))
	return &buf
}

func logLines(buf *bytes.Buffer) []string {
	trimmed := strings.TrimRight(buf.String(), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func linesWith(buf *bytes.Buffer, want string) []string {
	var out []string
	for _, line := range logLines(buf) {
		if strings.Contains(line, want) {
			out = append(out, line)
		}
	}
	return out
}

func drive(t *testing.T, d *daemon, event protocol.Event) {
	t.Helper()
	if resp := d.applyEvent(event); !resp.OK {
		t.Fatalf("%s %s: %s", event.Event, event.ID, resp.Error)
	}
}

func TestDefaultLevelKeepsAnUpdateBurstOffTheLog(t *testing.T) {
	d, _ := testDaemon(t)
	buf := captureLog(d, slog.LevelInfo)

	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "burst"})
	for range 1000 {
		drive(t, d, protocol.Event{Event: protocol.SessionUpdated, ID: "burst"})
	}
	lines := logLines(buf)
	if len(lines) != 1 {
		t.Fatalf("1000 updates produced %d log lines, want 1 for the start alone:\n%s", len(lines), buf)
	}
	if !strings.Contains(lines[0], "session.started") {
		t.Errorf("the one line is not the lifecycle transition: %q", lines[0])
	}
	if len(linesWith(buf, "session.updated")) != 0 {
		t.Errorf("session.updated reached the default level:\n%s", buf)
	}
}

func TestDebugLevelKeepsPerUpdateDetail(t *testing.T) {
	d, _ := testDaemon(t)
	buf := captureLog(d, slog.LevelDebug)

	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "burst"})
	for range 10 {
		drive(t, d, protocol.Event{Event: protocol.SessionUpdated, ID: "burst"})
	}

	updates := 0
	for _, line := range logLines(buf) {
		if strings.Contains(line, "session.updated") {
			updates++
		}
	}
	if updates != 10 {
		t.Errorf("debug level logged %d of 10 updates; the detail must be demoted, not lost", updates)
	}
}

func TestEveryLifecycleTransitionStaysAtInfo(t *testing.T) {
	for _, terminal := range []protocol.EventType{
		protocol.SessionCompleted,
		protocol.SessionFailed,
		protocol.SessionCancelled,
	} {
		t.Run(string(terminal), func(t *testing.T) {
			d, _ := testDaemon(t)
			buf := captureLog(d, slog.LevelInfo)

			drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "s"})
			drive(t, d, protocol.Event{Event: terminal, ID: "s"})

			if got := len(linesWith(buf, "session event")); got != 2 {
				t.Fatalf("%s produced %d lifecycle lines, want 2:\n%s", terminal, got, buf)
			}
		})
	}
}

type brokenRenderer struct {
	recorder
}

func (b *brokenRenderer) Update(harmony.State) error {
	return errors.New("audio device disappeared")
}

func TestAPersistentRendererFaultIsDeduplicated(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &brokenRenderer{}
	buf := captureLog(d, slog.LevelInfo)

	clock := time.Unix(0, 0)
	d.throttle.now = func() time.Time { return clock }

	d.applyEvent(protocol.Event{Event: protocol.SessionStarted, ID: "s"})
	for range 600 {
		d.applyEvent(protocol.Event{Event: protocol.SessionUpdated, ID: "s"})
		clock = clock.Add(100 * time.Millisecond)
	}

	faults := linesWith(buf, "renderer update failed")
	if len(faults) != 1 {
		t.Fatalf("601 failures inside one window produced %d lines, want 1:\n%s", len(faults), buf)
	}

	d.applyEvent(protocol.Event{Event: protocol.SessionUpdated, ID: "s"})

	faults = linesWith(buf, "renderer update failed")
	if len(faults) != 2 {
		t.Fatalf("the window elapsed and produced %d fault lines, want 2:\n%s", len(faults), buf)
	}
	if !strings.Contains(faults[1], "repeats=600") {
		t.Errorf("the second line does not report the suppressed repeats: %q", faults[1])
	}
}

func TestDistinctFaultsAreNotCoalesced(t *testing.T) {
	d, _ := testDaemon(t)
	buf := captureLog(d, slog.LevelInfo)

	d.rendererFailed("renderer update failed", errors.New("device gone"), "session", "a")
	d.rendererFailed("renderer update failed", errors.New("buffer underrun"), "session", "a")

	if got := len(logLines(buf)); got != 2 {
		t.Errorf("two different faults produced %d lines, want 2: coalescing unlike errors hides one of them:\n%s", got, buf)
	}
}

func TestTheThrottleForgetsWhenItsKeyspaceGrows(t *testing.T) {
	th := newThrottle(time.Hour)
	for i := range maxThrottleKeys {
		th.admit(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	if len(th.last) > maxThrottleKeys {
		t.Fatalf("throttle holds %d keys, above the %d bound", len(th.last), maxThrottleKeys)
	}

	th.admit("one more")
	if len(th.last) > maxThrottleKeys {
		t.Errorf("throttle grew to %d keys; a daemon running for months must not accumulate error strings", len(th.last))
	}
}

func TestSummaryReportsStateRatherThanEvents(t *testing.T) {
	d, _ := testDaemon(t)
	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "s"})
	for range 50 {
		drive(t, d, protocol.Event{Event: protocol.SessionUpdated, ID: "s"})
	}

	buf := captureLog(d, slog.LevelInfo)
	d.logSummary()

	lines := logLines(buf)
	if len(lines) != 1 {
		t.Fatalf("summary produced %d lines, want 1:\n%s", len(lines), buf)
	}
	for _, want := range []string{"sessions=1", "voices=1", "events=51", "dropped_phrases=0"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("summary is missing %q: %q", want, lines[0])
		}
	}
}

func TestSummaryCountersResetBetweenIntervals(t *testing.T) {
	d, _ := testDaemon(t)
	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "s"})

	buf := captureLog(d, slog.LevelInfo)
	d.logSummary()
	d.logSummary()

	lines := logLines(buf)
	if len(lines) != 2 {
		t.Fatalf("two summaries produced %d lines, want 2:\n%s", len(lines), buf)
	}
	if !strings.Contains(lines[1], "events=0") {
		t.Errorf("the second summary re-reports the first interval's events: %q", lines[1])
	}
}

func TestAnIdleDaemonWritesNothing(t *testing.T) {
	d, _ := testDaemon(t)
	buf := captureLog(d, slog.LevelInfo)

	d.logSummary()

	if buf.Len() != 0 {
		t.Errorf("an idle interval logged %q; a daemon meant to run continuously must be silent when nothing is happening", buf)
	}
}

func TestSummaryReportsSuppressedFaults(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &brokenRenderer{}
	buf := captureLog(d, slog.LevelInfo)

	clock := time.Unix(0, 0)
	d.throttle.now = func() time.Time { return clock }

	d.applyEvent(protocol.Event{Event: protocol.SessionStarted, ID: "s"})
	for range 5 {
		d.applyEvent(protocol.Event{Event: protocol.SessionUpdated, ID: "s"})
	}
	d.logSummary()

	summary := linesWith(buf, "soundscape")
	if len(summary) != 1 {
		t.Fatalf("expected one summary line, got %d:\n%s", len(summary), buf)
	}
	if !strings.Contains(summary[0], "suppressed=5") {
		t.Errorf("summary does not account for the suppressed faults: %q", summary[0])
	}
}

func TestSummaryCountsPhrasesTheRendererDropped(t *testing.T) {
	d, _ := testDaemon(t)
	d.render = &droppingRenderer{dropped: 7}
	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "s"})

	buf := captureLog(d, slog.LevelInfo)
	d.logSummary()

	if !strings.Contains(buf.String(), "dropped_phrases=7") {
		t.Errorf("summary does not report the renderer's dropped phrases: %q", buf)
	}
}

type droppingRenderer struct {
	recorder
	dropped int
}

func (r *droppingRenderer) DroppedPhrases() int { return r.dropped }

func TestARepeatedThemeFailureIsDeduplicated(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".hum"), 0o700); err != nil {
		t.Fatalf("mkdir .hum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".hum", "config.yaml"), []byte("music:\n  theme: missing\n"), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	d, _ := testDaemon(t)
	d.globalFile = filepath.Join(home, "config.yaml")
	buf := captureLog(d, slog.LevelInfo)

	clock := time.Unix(0, 0)
	d.throttle.now = func() time.Time { return clock }

	for i := range 200 {
		id := "s" + strconv.Itoa(i)
		drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: id, Root: project})
		drive(t, d, protocol.Event{Event: protocol.SessionCompleted, ID: id})
	}

	warnings := linesWith(buf, "keeping the current theme")
	if len(warnings) != 1 {
		t.Fatalf("200 sessions naming an unloadable theme produced %d warnings, want 1:\n%s", len(warnings), buf)
	}

	clock = clock.Add(2 * time.Minute)
	drive(t, d, protocol.Event{Event: protocol.SessionStarted, ID: "last", Root: project})

	warnings = linesWith(buf, "keeping the current theme")
	if len(warnings) != 2 {
		t.Fatalf("the window elapsed and produced %d warnings, want 2:\n%s", len(warnings), buf)
	}
	if !strings.Contains(warnings[1], "repeats=199") {
		t.Errorf("the second warning does not report the suppressed repeats: %q", warnings[1])
	}
}
