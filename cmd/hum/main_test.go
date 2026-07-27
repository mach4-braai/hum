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

// unreachableSocket points the client at a path where nothing listens.
func unreachableSocket(t *testing.T) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "absent.sock")
	t.Setenv("HUM_SOCKET", socket)
	return socket
}

// serveOne answers exactly one request with response. The returned await
// closes the listener and waits for the server goroutine, giving the caller a
// happens-before edge on the decoded request: socket traffic alone is not one,
// and the race detector rightly refuses to infer it.
func serveOne(t *testing.T, response string) func() protocol.Request {
	t.Helper()
	// Short path: a Unix socket address is capped near 104 bytes, and macOS
	// temp directories are already long.
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

	var got protocol.Request
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = json.NewDecoder(conn).Decode(&got)
		_, _ = io.WriteString(conn, response)
	}()

	// Closing before waiting, and idempotent: a test whose client never dials
	// would otherwise block in Accept forever.
	var once sync.Once
	await := func() protocol.Request {
		once.Do(func() {
			listener.Close()
			<-done
		})
		return got
	}
	t.Cleanup(func() { await() })
	return await
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

// The help text is the only place the exit codes are documented, so a caller
// scripting against them can discover the contract.
func TestHelpListsEveryDocumentedCommandAndExitCode(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}} {
		var stdout, stderr bytes.Buffer

		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("run(%v) = %d, want %d", args, code, exitUsage)
		}

		help := stderr.String()
		for _, command := range []string{
			"init", "start", "stop", "complete", "fail",
			"status", "mute", "doctor", "theme list", "theme use",
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
	cases := map[string][]string{
		"bare command": {"ping", "extra"},
		"theme list":   {"theme", "list", "extra"},
		"theme use":    {"theme", "use", "a", "b"},
		"theme alone":  {"theme"},
		"theme bogus":  {"theme", "restyle"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			if code := run(args, &stdout, &stderr); code != exitUsage {
				t.Errorf("run(%v) = %d, want %d", args, code, exitUsage)
			}
			if stderr.Len() == 0 {
				t.Errorf("run(%v) rejected the arguments silently", args)
			}
		})
	}
}

// Every command that talks to the daemon must report unreachability as 3, not
// as a generic failure: a CI script uses the difference to skip Hum entirely.
func TestCommandsExitThreeWhenTheDaemonIsAbsent(t *testing.T) {
	for _, args := range [][]string{
		{"ping"}, {"status"}, {"mute"}, {"stop"},
		{"theme", "list"}, {"theme", "use", "minimal"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			socket := unreachableSocket(t)
			var stdout, stderr bytes.Buffer

			code := run(args, &stdout, &stderr)

			if code != exitUnreachable {
				t.Errorf("run(%v) = %d, want %d", args, code, exitUnreachable)
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
		})
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

func TestThemeUseCarriesTheName(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"theme", "use", "orchestra"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}

	got := await()
	if got.Command != protocol.CmdThemeUse {
		t.Errorf("daemon received command %q, want %q", got.Command, protocol.CmdThemeUse)
	}
	if got.Value != "orchestra" {
		t.Errorf("daemon received value %q, want %q", got.Value, "orchestra")
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

	code := run([]string{"theme", "use", "absent"}, &stdout, &stderr)

	if code != exitDaemonError {
		t.Errorf("exit code = %d, want %d", code, exitDaemonError)
	}
	if got := stderr.String(); !strings.Contains(got, "no such theme") {
		t.Errorf("stderr = %q, want the daemon's message", got)
	}
}

// A failure with no message must still say something actionable.
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

func TestSuccessfulDataIsPrintedWithoutJSONFlag(t *testing.T) {
	serveOne(t, `{"ok":true,"data":{"sessions":2}}`+"\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"status"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr = %q", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sessions") {
		t.Errorf("stdout = %q, want the response payload", stdout.String())
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

// Flags read naturally on either side of the command, so neither spelling is a
// usage error the user has to discover.
func TestFlagsAreAcceptedAfterTheCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "ping"},
		{"ping", "--json"},
		{"theme", "use", "minimal", "--json"},
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

// The multiword forms take flags in every position a user might type them.
func TestThemeAcceptsFlagsAroundItsSubcommand(t *testing.T) {
	for _, args := range [][]string{
		{"theme", "--json", "list"},
		{"theme", "list", "--json"},
		{"theme", "use", "minimal", "--json"},
		{"theme", "--json", "use", "minimal"},
		{"theme", "use", "--json", "minimal"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			serveOne(t, `{"ok":true}`+"\n")
			var stdout, stderr bytes.Buffer

			if code := run(args, &stdout, &stderr); code != exitOK {
				t.Fatalf("run(%v) = %d, want %d; stderr = %q", args, code, exitOK, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Errorf("run(%v) printed nothing, want --json honoured", args)
			}
		})
	}
}

func TestUnknownFlagsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"ping", "--bogus"},
		{"theme", "--bogus", "list"},
		{"theme", "list", "--bogus"},
		{"theme", "use"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			if code := run(args, &stdout, &stderr); code != exitUsage {
				t.Errorf("run(%v) = %d, want %d", args, code, exitUsage)
			}
			if stderr.Len() == 0 {
				t.Errorf("run(%v) failed silently", args)
			}
		})
	}
}

// An envelope that cannot be marshalled must never reach the socket, and the
// caller must not be told the request succeeded.
func TestSendRefusesAnUnmarshallableRequest(t *testing.T) {
	await := serveOne(t, `{"ok":true}`+"\n")
	var stdout, stderr bytes.Buffer

	code := send(protocol.Request{}, defaultTimeout, false, &stdout, &stderr)

	if code != exitUnreachable {
		t.Errorf("send() = %d, want %d", code, exitUnreachable)
	}
	if got := await(); got.Command != "" {
		t.Errorf("daemon received %q, want nothing sent", got.Command)
	}
}

// Valid JSON that is not a response object: the decode of the envelope fails
// after the raw message has already been read.
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
