package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestTerminalCommandsEmitTheirOwnEvent(t *testing.T) {
	for command, want := range map[string]protocol.EventType{
		"complete": protocol.SessionCompleted,
		"fail":     protocol.SessionFailed,
		"cancel":   protocol.SessionCancelled,
	} {
		t.Run(command, func(t *testing.T) {
			await := serveOne(t, `{"ok":true}`+"\n")
			var stdout, stderr bytes.Buffer

			code := run([]string{command, "--id", "t1"}, &stdout, &stderr)

			if code != exitOK {
				t.Fatalf("exit %d want %d; stderr=%q", code, exitOK, stderr.String())
			}
			got := await()
			if got.Event == nil {
				t.Fatal("daemon received a command request, want an event")
			}
			if got.Event.Event != want {
				t.Errorf("event = %q, want %q", got.Event.Event, want)
			}
			if got.Event.ID != "t1" {
				t.Errorf("id = %q, want %q", got.Event.ID, "t1")
			}
		})
	}
}

func TestTerminalCommandsTakeTheIDFromTheEnvironment(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	t.Setenv(envSessionID, "from-env")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"complete"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if got := await(); got.Event == nil || got.Event.ID != "from-env" {
		t.Errorf("id = %+v, want %q", got.Event, "from-env")
	}
}

func TestTerminalCommandsRefuseToInventAnID(t *testing.T) {
	t.Setenv(envSessionID, "")
	var stdout, stderr bytes.Buffer

	code := run([]string{"complete"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), envSessionID) {
		t.Errorf("stderr = %q, want it to name %s", stderr.String(), envSessionID)
	}
}

func TestTerminalCommandsReportADaemonRefusal(t *testing.T) {
	serveOne(t, `{"ok":false,"error":"session is already terminal"}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"complete", "--id", "t1"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Fatalf("exit %d want %d", code, exitDaemonError)
	}
	if !strings.Contains(stderr.String(), "already terminal") {
		t.Errorf("stderr = %q, want the daemon's message", stderr.String())
	}
}

func TestTerminalCommandsRejectAnOperand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"cancel", "--id", "t1", "extra"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "extra") {
		t.Errorf("stderr = %q, want it to name the operand", stderr.String())
	}
}

func TestTerminalCommandsRejectAnUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"fail", "--reason", "flaky"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d want %d", code, exitUsage)
	}
}
