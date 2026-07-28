package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

func TestStartSendsCorrectEvent(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"start", "--id", "t1", "--title", "build", "--workspace", "tofu"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit %d want %d; stderr=%q", code, exitOK, stderr.String())
	}
	got := await()
	if got.Event == nil {
		t.Fatal("daemon received a command request, want an event")
	}
	if got.Event.Event != protocol.SessionStarted {
		t.Errorf("event type = %q, want %q", got.Event.Event, protocol.SessionStarted)
	}
	if got.Event.ID != "t1" {
		t.Errorf("id = %q, want %q", got.Event.ID, "t1")
	}
	if got.Event.Title != "build" {
		t.Errorf("title = %q, want %q", got.Event.Title, "build")
	}
	if got.Event.Workspace != "tofu" {
		t.Errorf("workspace = %q, want %q", got.Event.Workspace, "tofu")
	}
	if !filepath.IsAbs(got.Event.Root) {
		t.Errorf("root = %q, want an absolute path", got.Event.Root)
	}
	if want := "t1\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestStartGeneratesIDWhenAbsent(t *testing.T) {
	await := serveResponses(t, `{"ok":true}`+"\n", `{"ok":true}`+"\n")

	var stdout1, stderr1 bytes.Buffer
	code1 := run([]string{"start"}, &stdout1, &stderr1)
	if code1 != exitOK {
		t.Fatalf("run1 exit %d; stderr=%q", code1, stderr1.String())
	}

	var stdout2, stderr2 bytes.Buffer
	code2 := run([]string{"start"}, &stdout2, &stderr2)
	if code2 != exitOK {
		t.Fatalf("run2 exit %d; stderr=%q", code2, stderr2.String())
	}

	reqs := await()
	if len(reqs) != 2 {
		t.Fatalf("daemon received %d requests, want 2", len(reqs))
	}

	out1 := stdout1.String()
	if n := strings.Count(out1, "\n"); n != 1 {
		t.Errorf("run1: stdout has %d newlines, want exactly 1; stdout=%q", n, out1)
	}
	id1 := strings.TrimRight(out1, "\n")
	if id1 == "" {
		t.Error("run1: stdout is empty, want the session id")
	}

	out2 := stdout2.String()
	id2 := strings.TrimRight(out2, "\n")

	if reqs[0].Event == nil || reqs[0].Event.ID != id1 {
		t.Errorf("run1: stdout id %q != daemon id %q", id1, startEventID(reqs[0]))
	}
	if reqs[1].Event == nil || reqs[1].Event.ID != id2 {
		t.Errorf("run2: stdout id %q != daemon id %q", id2, startEventID(reqs[1]))
	}
	if id1 == id2 {
		t.Errorf("expected different ids on two runs; both were %q", id1)
	}
}

func startEventID(r protocol.Request) string {
	if r.Event == nil {
		return ""
	}
	return r.Event.ID
}

func TestStartEnvSessionIDIsUsed(t *testing.T) {
	t.Setenv("HUM_SESSION_ID", "env-id")
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout bytes.Buffer

	code := run([]string{"start"}, &stdout, &bytes.Buffer{})

	if code != exitOK {
		t.Fatalf("exit %d want %d", code, exitOK)
	}
	got := await()
	if got.Event == nil || got.Event.ID != "env-id" {
		t.Errorf("daemon id = %q, want %q", startEventID(got), "env-id")
	}
	if want := "env-id\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestStartFlagIDWinsOverEnv(t *testing.T) {
	t.Setenv("HUM_SESSION_ID", "env-id")
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout bytes.Buffer

	code := run([]string{"start", "--id", "flag-id"}, &stdout, &bytes.Buffer{})

	if code != exitOK {
		t.Fatalf("exit %d want %d", code, exitOK)
	}
	got := await()
	if got.Event == nil || got.Event.ID != "flag-id" {
		t.Errorf("daemon id = %q, want %q", startEventID(got), "flag-id")
	}
	if want := "flag-id\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestStartMetaAccumulates(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")

	code := run([]string{"start", "--id", "t1", "--meta", "a=1", "--meta", "b=2"}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitOK {
		t.Fatalf("exit %d want %d", code, exitOK)
	}
	got := await()
	if got.Event == nil {
		t.Fatal("got command, want event")
	}
	if got.Event.Metadata["a"] != "1" {
		t.Errorf("metadata[a] = %q, want %q", got.Event.Metadata["a"], "1")
	}
	if got.Event.Metadata["b"] != "2" {
		t.Errorf("metadata[b] = %q, want %q", got.Event.Metadata["b"], "2")
	}
}

func TestStartMetaBadPairIsUsageError(t *testing.T) {
	await := serveResponses(t)

	code := run([]string{"start", "--meta", "bad"}, &bytes.Buffer{}, &bytes.Buffer{})

	reqs := await()
	if code != exitUsage {
		t.Errorf("exit %d want %d", code, exitUsage)
	}
	if len(reqs) != 0 {
		t.Errorf("daemon received %d requests, want 0", len(reqs))
	}
}

func TestStartMetaEmptyKeyIsUsageError(t *testing.T) {
	await := serveResponses(t)

	code := run([]string{"start", "--meta", "=value"}, &bytes.Buffer{}, &bytes.Buffer{})

	reqs := await()
	if code != exitUsage {
		t.Errorf("exit %d want %d", code, exitUsage)
	}
	if len(reqs) != 0 {
		t.Errorf("daemon received %d requests, want 0", len(reqs))
	}
}

func TestStartPriorityNonIntIsUsageError(t *testing.T) {
	await := serveResponses(t)

	code := run([]string{"start", "--priority", "x"}, &bytes.Buffer{}, &bytes.Buffer{})

	reqs := await()
	if code != exitUsage {
		t.Errorf("exit %d want %d", code, exitUsage)
	}
	if len(reqs) != 0 {
		t.Errorf("daemon received %d requests, want 0", len(reqs))
	}
}

func TestStartRootWithHumConfigDir(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	projectDir := t.TempDir()
	humDir := filepath.Join(projectDir, ".hum")
	if err := os.MkdirAll(humDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(humDir, "config.yaml"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	await := serveOne(t, `{"ok":true}`+"\n")

	code := run([]string{"start", "--id", "t1", "--root", projectDir}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitOK {
		t.Fatalf("exit %d want %d", code, exitOK)
	}
	got := await()
	want, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Event == nil || got.Event.Root != want {
		t.Errorf("root = %q, want %q", startEventRoot(got), want)
	}
}

func startEventRoot(r protocol.Request) string {
	if r.Event == nil {
		return ""
	}
	return r.Event.Root
}

func TestStartRootNonexistentIsUsageError(t *testing.T) {
	unreachableSocket(t)

	code := run([]string{"start", "--root", "/nonexistent/hum-test-path-xyz"}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitUsage {
		t.Errorf("exit %d want %d", code, exitUsage)
	}
}

func TestStartFlagsAfterOperandParse(t *testing.T) {
	serveOne(t, `{"ok":true}`+"\n")
	var stdout bytes.Buffer

	code := run([]string{"start", "--id", "t1", "--json"}, &stdout, &bytes.Buffer{})

	if code != exitOK {
		t.Errorf("exit %d want %d", code, exitOK)
	}
}

func TestStartStrayArgumentIsUsageError(t *testing.T) {
	unreachableSocket(t)

	code := run([]string{"start", "extra"}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitUsage {
		t.Errorf("exit %d want %d", code, exitUsage)
	}
}

func TestStartAbsentDaemonExitsThree(t *testing.T) {
	unreachableSocket(t)

	code := run([]string{"start", "--id", "t1"}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitUnreachable {
		t.Errorf("exit %d want %d", code, exitUnreachable)
	}
}

func TestStartDaemonErrorExitsOne(t *testing.T) {
	serveOne(t, `{"ok":false,"error":"session collision"}`+"\n")

	code := run([]string{"start", "--id", "t1"}, &bytes.Buffer{}, &bytes.Buffer{})

	if code != exitDaemonError {
		t.Errorf("exit %d want %d", code, exitDaemonError)
	}
}

func TestStartEndToEnd(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d := startHumd(t)

	code, out := runBinary(t, os.Getenv("HUM_SOCKET"), "start", "--id", "t1", "--title", "build", "--workspace", "tofu")

	if code != exitOK {
		t.Fatalf("exit %d want %d; output=%s", code, exitOK, out)
	}
	logs := d.logs()
	if !strings.Contains(logs, "voices=1") {
		t.Errorf("daemon logs missing voices=1; logs:\n%s", logs)
	}
}
