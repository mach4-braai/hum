package main

import (
	"bytes"
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

func unreachableSocket(t *testing.T) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "absent.sock")
	t.Setenv("HUM_SOCKET", socket)
	return socket
}

func serveResponses(t *testing.T, responses ...string) func() []protocol.Request {
	t.Helper()
	dir, err := os.MkdirTemp("", "hum")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Setenv("HUM_SOCKET", socket)

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
	return await
}

func serveOne(t *testing.T, response string) func() protocol.Request {
	t.Helper()
	await := serveResponses(t, response)
	return func() protocol.Request {
		requests := await()
		if len(requests) == 0 {
			return protocol.Request{}
		}
		return requests[0]
	}
}

func TestRunWithoutArgumentsPrintsUsageToStderrAndExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty so callers can capture output cleanly", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "usage:") {
		t.Errorf("stderr = %q, want it to contain %q", got, "usage:")
	}
}

func TestHelpListsEveryDocumentedCommandAndExitCode(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}} {
		var stdout, stderr bytes.Buffer

		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("run(%v) = %d, want %d", args, code, exitUsage)
		}

		help := stderr.String()
		for _, command := range []string{
			"init", "start", "stop", "complete", "fail",
			"status", "mute", "unmute", "volume", "doctor", "theme list", "theme use",
		} {
			if !strings.Contains(help, command) {
				t.Errorf("run(%v) help does not list %q", args, command)
			}
		}
		for _, code := range []string{"0", "1", "2", "3"} {
			if !strings.Contains(help, "  "+code+"  ") {
				t.Errorf("run(%v) help does not document exit code %s", args, code)
			}
		}
	}
}

func TestEveryHelpedCommandIsRegistered(t *testing.T) {
	for _, name := range []string{
		"init", "start", "stop", "status", "mute", "unmute",
		"volume", "doctor", "theme", "ping",
	} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q appears in the usage text but is not registered", name)
		}
	}
}

func TestRegisteringACommandTwicePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("register did not panic on a duplicate name, so a command could be silently shadowed")
		}
	}()

	register("ping", runPing)
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"bogus"}, &stdout, &stderr)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if got := stderr.String(); !strings.Contains(got, `unknown command "bogus"`) {
		t.Errorf("stderr = %q, want it to name the unknown command", got)
	}
}

func TestUnparseableFlagIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"--timeout", "soon", "ping"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}

func TestTrailingOperandsAreRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping", "extra"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if stderr.Len() == 0 {
		t.Error("the extra argument was rejected silently")
	}
}

func TestAnAbsentDaemonIsReportedActionably(t *testing.T) {
	socket := unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"ping"}, &stdout, &stderr)

	if code != exitUnreachable {
		t.Errorf("exit code = %d, want %d", code, exitUnreachable)
	}
	message := stderr.String()
	if !strings.Contains(message, socket) {
		t.Errorf("stderr = %q, want the socket path %q named", message, socket)
	}
	if !strings.Contains(message, "humd") {
		t.Errorf("stderr = %q, want it to say how to start the daemon", message)
	}
	if strings.Contains(message, "connect:") || strings.Contains(message, "dial") {
		t.Errorf("stderr = %q, want an actionable line rather than the raw dial error", message)
	}
}

func TestPingSendsTheControlCommandAndSucceeds(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"ping"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	got := await()
	if got.Command != protocol.CmdPing {
		t.Errorf("daemon received command %q, want %q", got.Command, protocol.CmdPing)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on success", stderr.String())
	}
}

func TestJSONPrintsTheRawResponseOnStdoutOnly(t *testing.T) {
	serveOne(t, `{"ok":true,"data":{"sessions":2}}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"--json", "ping"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty so --json output can be piped", stderr.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("stdout = %q, want parseable JSON: %v", stdout.String(), err)
	}
	if parsed["ok"] != true {
		t.Errorf("stdout = %q, want the daemon's own response", stdout.String())
	}
}

func TestDaemonFailureExitsOne(t *testing.T) {
	serveOne(t, `{"ok":false,"error":"no such theme"}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"ping"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit code = %d, want %d", code, exitDaemonError)
	}
	if got := stderr.String(); !strings.Contains(got, "no such theme") {
		t.Errorf("stderr = %q, want the daemon's message", got)
	}
}

func TestSilentDaemonFailureStillExplainsItself(t *testing.T) {
	serveOne(t, `{"ok":false}`+"\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping"}, &stdout, &stderr); code != exitDaemonError {
		t.Errorf("exit code = %d, want %d", code, exitDaemonError)
	}
	if strings.TrimSpace(stderr.String()) == "hum:" {
		t.Errorf("stderr = %q, want an explanation rather than a bare prefix", stderr.String())
	}
}

func TestMalformedResponseIsUnreachableNotSuccess(t *testing.T) {
	serveOne(t, "not json\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping"}, &stdout, &stderr); code != exitUnreachable {
		t.Errorf("exit code = %d, want %d", code, exitUnreachable)
	}
}

func TestClosedConnectionIsUnreachable(t *testing.T) {
	serveOne(t, "")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping"}, &stdout, &stderr); code != exitUnreachable {
		t.Errorf("exit code = %d, want %d", code, exitUnreachable)
	}
}

func TestNonObjectResponseIsUnreachable(t *testing.T) {
	serveOne(t, "42\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping"}, &stdout, &stderr); code != exitUnreachable {
		t.Errorf("exit code = %d, want %d", code, exitUnreachable)
	}
	if !strings.Contains(stderr.String(), "malformed response") {
		t.Errorf("stderr = %q, want it to name a malformed response", stderr.String())
	}
}

func TestMainExitsWithRunCodeAndWritesUsageToStderr(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr, origExit := os.Stderr, exit
	t.Cleanup(func() { os.Stderr, exit = origStderr, origExit })
	var code int
	exited := false
	os.Stderr = w
	exit = func(c int) { code, exited = c, true }

	main()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}

	if !exited {
		t.Fatal("main returned without exiting; the process would report success")
	}
	if code != exitUsage {
		t.Errorf("main exit code = %d, want %d from run", code, exitUsage)
	}
	if !strings.Contains(string(out), "usage:") {
		t.Errorf("main stderr = %q, want it to contain %q", out, "usage:")
	}
}

func TestFlagsAreAcceptedAfterTheCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "ping"},
		{"ping", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			serveOne(t, `{"ok":true}`+"\n")
			var stdout, stderr bytes.Buffer

			if code := run(args, &stdout, &stderr); code != exitOK {
				t.Fatalf("run(%v) = %d, want %d; stderr = %q", args, code, exitOK, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Errorf("run(%v) printed nothing, want --json to have been honoured", args)
			}
		})
	}
}

func TestTimeoutAfterTheCommandIsParsed(t *testing.T) {
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping", "--timeout", "50ms"}, &stdout, &stderr); code != exitUnreachable {
		t.Errorf("exit code = %d, want %d", code, exitUnreachable)
	}
	if strings.Contains(stderr.String(), "not defined") {
		t.Errorf("stderr = %q, want --timeout accepted after the command", stderr.String())
	}
}

func TestUnknownFlagsAreUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"ping", "--bogus"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if stderr.Len() == 0 {
		t.Error("the unknown flag was rejected silently")
	}
}

func TestSendRefusesAnUnmarshallableRequest(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer
	e := &env{stdout: &stdout, stderr: &stderr, opts: &options{timeout: defaultTimeout}}

	code := send(e, protocol.Request{})

	if code != exitUnreachable {
		t.Errorf("send() = %d, want %d", code, exitUnreachable)
	}
	if got := await(); got.Command != "" {
		t.Errorf("daemon received %q, want nothing sent", got.Command)
	}
}

func TestFetchPayloadReportsAPayloadItCannotDecode(t *testing.T) {
	serveOne(t, `{"ok":true,"data":{"sessions":"many"}}`+"\n")
	var stdout, stderr bytes.Buffer
	e := &env{stdout: &stdout, stderr: &stderr, opts: &options{timeout: defaultTimeout}}

	_, _, code := fetchStatus(e)

	if code != exitUnreachable {
		t.Errorf("fetchStatus() = %d, want %d", code, exitUnreachable)
	}
	if !strings.Contains(stderr.String(), "malformed status payload") {
		t.Errorf("stderr = %q, want it to name the malformed payload", stderr.String())
	}
}

func TestPersistReportsAConfigItCannotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("- not a mapping\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var stdout, stderr bytes.Buffer
	e := &env{stdout: &stdout, stderr: &stderr, opts: &options{timeout: defaultTimeout}}

	code := persist(e, map[string]string{"audio.muted": "true"})

	if code != exitDaemonError {
		t.Errorf("persist() = %d, want %d", code, exitDaemonError)
	}
	if !strings.Contains(stderr.String(), "cannot update") {
		t.Errorf("stderr = %q, want it to say the config could not be updated", stderr.String())
	}
}
