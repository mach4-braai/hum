package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func fakeDaemon(t *testing.T, responses ...string) (string, func() []protocol.Request) {
	t.Helper()

	dir, err := os.MkdirTemp("", "hb")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var mu sync.Mutex
	var got []protocol.Request
	done := make(chan struct{})
	go func() {
		defer close(done)
		for served := 0; ; served++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var request protocol.Request
			_ = json.NewDecoder(conn).Decode(&request)
			mu.Lock()
			got = append(got, request)
			mu.Unlock()
			if served < len(responses) {
				_, _ = io.WriteString(conn, responses[served])
			}
			conn.Close()
		}
	}()

	var once sync.Once
	await := func() []protocol.Request {
		once.Do(func() {
			listener.Close()
			<-done
		})
		mu.Lock()
		defer mu.Unlock()
		return append([]protocol.Request(nil), got...)
	}
	t.Cleanup(func() { await() })
	return socket, await
}

const noSessionInEnvironment = "HUM_SESSION_ID="

func TestBinaryLifecycleCommandsSucceed(t *testing.T) {
	for _, command := range []string{"complete", "fail", "cancel", "update"} {
		t.Run(command, func(t *testing.T) {
			socket, await := fakeDaemon(t, `{"ok":true}`+"\n")

			code, out := runBinaryWith(t, socket, nil, command, "--id", "t1")

			if code != exitOK {
				t.Fatalf("hum %s exited %d, want %d\n%s", command, code, exitOK, out)
			}
			requests := await()
			if len(requests) != 1 || requests[0].Event == nil {
				t.Fatalf("daemon received %+v, want one event", requests)
			}
			if requests[0].Event.ID != "t1" {
				t.Errorf("id = %q, want t1", requests[0].Event.ID)
			}
		})
	}
}

func TestBinaryLifecycleExitCodes(t *testing.T) {
	refusal := `{"ok":false,"error":"session is already terminal"}` + "\n"

	cases := []struct {
		name      string
		responses []string
		absent    bool
		args      []string
		want      int
	}{
		{"refused completion", []string{refusal}, false, []string{"complete", "--id", "t1"}, exitDaemonError},
		{"refused update", []string{refusal}, false, []string{"update", "--id", "t1"}, exitDaemonError},
		{"cancel without an id", nil, false, []string{"cancel"}, exitUsage},
		{"update without an id", nil, false, []string{"update"}, exitUsage},
		{"update with malformed metadata", nil, false, []string{"update", "--id", "t1", "--meta", "bad"}, exitUsage},
		{"fail with an operand", nil, false, []string{"fail", "--id", "t1", "extra"}, exitUsage},
		{"complete without a daemon", nil, true, []string{"complete", "--id", "t1"}, exitUnreachable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "absent.sock")
			if !tc.absent {
				socket, _ = fakeDaemon(t, tc.responses...)
			}

			code, out := runBinaryWith(t, socket, []string{noSessionInEnvironment}, tc.args...)

			if code != tc.want {
				t.Errorf("hum %v exited %d, want %d\n%s", tc.args, code, tc.want, out)
			}
		})
	}
}

func TestBinaryTerminalRefusalRepeatsTheDaemonsReason(t *testing.T) {
	socket, _ := fakeDaemon(t, `{"ok":false,"error":"unknown session id"}`+"\n")

	code, out := runBinaryWith(t, socket, nil, "fail", "--id", "ghost")

	if code != exitDaemonError {
		t.Fatalf("hum fail exited %d, want %d\n%s", code, exitDaemonError, out)
	}
	if !strings.Contains(out, "unknown session id") {
		t.Errorf("output = %q, want the daemon's own message", out)
	}
}

func TestBinaryMissingIDNamesTheEnvironmentVariable(t *testing.T) {
	socket, _ := fakeDaemon(t)

	code, out := runBinaryWith(t, socket, []string{noSessionInEnvironment}, "complete")

	if code != exitUsage {
		t.Fatalf("hum complete exited %d, want %d\n%s", code, exitUsage, out)
	}
	if !strings.Contains(out, envSessionID) {
		t.Errorf("output = %q, want it to name %s", out, envSessionID)
	}
}
