package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestUpdateSendsMetadataWithoutEndingTheSession(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"update", "--id", "t1", "--meta", "agents=3", "--meta", "phase=plan"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d want %d; stderr=%q", code, exitOK, stderr.String())
	}
	got := await()
	if got.Event == nil {
		t.Fatal("daemon received a command request, want an event")
	}
	if got.Event.Event != protocol.SessionUpdated {
		t.Errorf("event = %q, want %q", got.Event.Event, protocol.SessionUpdated)
	}
	if got.Event.Metadata["agents"] != "3" || got.Event.Metadata["phase"] != "plan" {
		t.Errorf("metadata = %v, want agents=3 and phase=plan", got.Event.Metadata)
	}
	if stdout.String() != "" {
		t.Errorf("stdout = %q, want nothing: update reports no new id", stdout.String())
	}
}

func TestUpdateRejectsMetadataWithoutAValue(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"update", "--id", "t1", "--meta", "bad"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d want %d; stderr=%q", code, exitUsage, stderr.String())
	}
}

func TestUpdateRefusesToInventAnID(t *testing.T) {
	t.Setenv(envSessionID, "")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"update"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d want %d", code, exitUsage)
	}
}

func TestUpdateRejectsAnOperand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"update", "--id", "t1", "extra"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit %d want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "extra") {
		t.Errorf("stderr = %q, want it to name the operand", stderr.String())
	}
}

func TestUpdateCarriesATitleWhenGiven(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"update", "--id", "t1", "--title", "phase two"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d want %d; stderr=%q", code, exitOK, stderr.String())
	}
	if got := await(); got.Event == nil || got.Event.Title != "phase two" {
		t.Errorf("title = %+v, want %q", got.Event, "phase two")
	}
}
