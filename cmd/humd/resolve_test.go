package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/protocol"
)

func TestHandleRejectsAnInvalidRequestBeforeResolving(t *testing.T) {
	d, _ := testDaemon(t)

	resp := d.handle(protocol.Request{Command: protocol.Command("bogus")})
	if resp.OK {
		t.Fatal("handle(invalid) = ok, want a rejection")
	}
}

func (d *daemon) resolveForEvent(event protocol.Event) contextResolution {
	if event.Event != protocol.SessionStarted {
		return contextResolution{}
	}
	return resolveContext(d.globalFile, event.Root)
}

type asyncResult struct {
	response protocol.Response
	err      error
}

func sendAsync(socket string, request protocol.Request) <-chan asyncResult {
	result := make(chan asyncResult, 1)
	go func() {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			result <- asyncResult{err: err}
			return
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(daemonStopGrace)); err != nil {
			result <- asyncResult{err: err}
			return
		}
		if err := json.NewEncoder(conn).Encode(request); err != nil {
			result <- asyncResult{err: err}
			return
		}
		var response protocol.Response
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			result <- asyncResult{err: err}
			return
		}
		result <- asyncResult{response: response}
	}()
	return result
}

func stubBlockingResolver(t *testing.T) (releaseNow func(), entered *int32) {
	t.Helper()

	original := resolveSessionContext
	t.Cleanup(func() { resolveSessionContext = original })

	release := make(chan struct{})
	entered = new(int32)
	var once sync.Once
	releaseNow = func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseNow)

	resolveSessionContext = func(globalFile, root string) (*config.Config, string, error) {
		atomic.AddInt32(entered, 1)
		<-release
		return original(globalFile, root)
	}
	return releaseNow, entered
}

func waitForEntries(t *testing.T, entered *int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(daemonStopGrace)
	for atomic.LoadInt32(entered) < want {
		if time.Now().After(deadline) {
			t.Fatalf("resolver entered %d times, want %d", atomic.LoadInt32(entered), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBlockedSessionRootDoesNotStallOtherRequests(t *testing.T) {
	releaseNow, entered := stubBlockingResolver(t)

	d, _ := testDaemon(t)
	d.resolveTimeout = daemonStopGrace
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	blocked := sendAsync(socket, protocol.Request{
		Event: &protocol.Event{Event: protocol.SessionStarted, ID: "blocked", Root: t.TempDir()},
	})
	waitForEntries(t, entered, 1)

	if status := statusOf(t, socket); len(status.Sessions) != 0 {
		t.Errorf("status reported %d sessions while the start was still resolving, want none", len(status.Sessions))
	}

	releaseNow()
	result := <-blocked
	if result.err != nil {
		t.Fatalf("blocked session.started: %v", result.err)
	}
	if !result.response.OK {
		t.Fatalf("session.started after release = %q, want ok", result.response.Error)
	}
}

func TestSessionRootPastTheBoundFailsTheStart(t *testing.T) {
	_, entered := stubBlockingResolver(t)

	d, _ := testDaemon(t)
	d.resolveTimeout = 50 * time.Millisecond
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	root := t.TempDir()
	result := <-sendAsync(socket, protocol.Request{
		Event: &protocol.Event{Event: protocol.SessionStarted, ID: "blocked", Root: root},
	})
	if result.err != nil {
		t.Fatalf("blocked session.started: %v", result.err)
	}
	if result.response.OK {
		t.Fatal("blocked session.started = ok, want a timeout failure")
	}
	if !strings.Contains(result.response.Error, root) {
		t.Errorf("error %q does not name the blocked root %q", result.response.Error, root)
	}
	if !strings.Contains(result.response.Error, d.resolveTimeout.String()) {
		t.Errorf("error %q does not name the %s bound", result.response.Error, d.resolveTimeout)
	}
	waitForEntries(t, entered, 1)

	if status := statusOf(t, socket); len(status.Sessions) != 0 {
		t.Errorf("status reported %d sessions after the timed-out start, want none", len(status.Sessions))
	}
}

func TestTooManyOutstandingResolutionsFailFastThenRecover(t *testing.T) {
	releaseNow, entered := stubBlockingResolver(t)

	d, _ := testDaemon(t)
	d.resolveTimeout = 2 * time.Second
	socket, signals, done := startDaemon(t, d)
	t.Cleanup(func() {
		signals <- syscall.SIGTERM
		<-done
	})

	blocked := make([]<-chan asyncResult, maxConcurrentResolutions)
	for i := range blocked {
		blocked[i] = sendAsync(socket, protocol.Request{
			Event: &protocol.Event{Event: protocol.SessionStarted, ID: fmt.Sprintf("blocked%d", i), Root: t.TempDir()},
		})
	}

	deadline := time.Now().Add(daemonStopGrace)
	for atomic.LoadInt32(entered) < int32(maxConcurrentResolutions) {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d resolutions entered the resolver", atomic.LoadInt32(entered), maxConcurrentResolutions)
		}
		time.Sleep(time.Millisecond)
	}

	overflow := <-sendAsync(socket, protocol.Request{
		Event: &protocol.Event{Event: protocol.SessionStarted, ID: "overflow", Root: t.TempDir()},
	})
	if overflow.err != nil {
		t.Fatalf("overflow request: %v", overflow.err)
	}
	if overflow.response.OK {
		t.Fatal("overflow session.started = ok, want a refusal while every slot is held")
	}
	if !strings.Contains(overflow.response.Error, "too many") {
		t.Errorf("error %q does not say too many roots are resolving", overflow.response.Error)
	}

	releaseNow()

	for _, b := range blocked {
		if result := <-b; result.err != nil {
			t.Fatalf("blocked request: %v", result.err)
		}
	}

	deadline = time.Now().Add(daemonStopGrace)
	for len(d.resolveSlots) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("resolve slots never drained after release")
		}
		time.Sleep(time.Millisecond)
	}

	recovered := <-sendAsync(socket, protocol.Request{
		Event: &protocol.Event{Event: protocol.SessionStarted, ID: "recovered", Root: t.TempDir()},
	})
	if recovered.err != nil {
		t.Fatalf("recovered request: %v", recovered.err)
	}
	if !recovered.response.OK {
		t.Errorf("session.started after slots freed = %+v, want ok", recovered.response)
	}
}
