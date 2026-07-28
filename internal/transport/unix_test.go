package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/transport"
)

func okHandler(req protocol.Request) protocol.Response {
	return protocol.Response{OK: true}
}

func tmpDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "humtr")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func sockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(tmpDir(t), "t.sock")
}

func mustListen(t *testing.T, path string, opts transport.Options) transport.Listener {
	t.Helper()
	ln, err := transport.NewUnixListener(path, opts)
	if err != nil {
		t.Fatalf("NewUnixListener: %v", err)
	}
	return ln
}

type serverHandle struct {
	ln     transport.Listener
	cancel context.CancelFunc
	errCh  chan error
}

func startServer(t *testing.T, path string, h transport.Handler) serverHandle {
	t.Helper()
	ln := mustListen(t, path, transport.Options{
		Deadline: 2 * time.Second,
		Grace:    2 * time.Second,
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, h)
	}()
	waitReady(t, path)
	t.Cleanup(func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Error("Serve did not stop")
		}
		ln.Close()
	})
	return serverHandle{ln: ln, cancel: cancel, errCh: errCh}
}

func waitReady(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("server not ready within 2s")
}

func ping(t *testing.T, path string) protocol.Response {
	t.Helper()
	return sendOne(t, path, protocol.Request{Command: protocol.CmdPing})
}

func sendOne(t *testing.T, path string, req protocol.Request) protocol.Response {
	t.Helper()
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func trySendOne(path string, req protocol.Request) (protocol.Response, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return protocol.Response{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return protocol.Response{}, err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return protocol.Response{}, err
	}
	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return protocol.Response{}, err
	}
	return resp, nil
}

func createStaleSocket(t *testing.T, path string) {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("syscall.Socket: %v", err)
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		syscall.Close(fd)
		t.Fatalf("syscall.Bind: %v", err)
	}
	syscall.Close(fd)
}

func TestPing(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	resp := ping(t, path)
	if !resp.OK {
		t.Fatalf("ping: want ok=true, got ok=false error=%q", resp.Error)
	}
}

func TestAddr(t *testing.T) {
	path := sockPath(t)
	srv := startServer(t, path, okHandler)
	if got := srv.ln.Addr(); got != path {
		t.Fatalf("Addr: want %q, got %q", path, got)
	}
}

func TestSocketMode(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode: want 0600, got %04o", got)
	}
}

func TestRelativePathRejected(t *testing.T) {
	_, err := transport.NewUnixListener("relative/path.sock", transport.Options{})
	if !errors.Is(err, transport.ErrRelativeSocket) {
		t.Fatalf("want ErrRelativeSocket, got %v", err)
	}
}

func TestConcurrentClients(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	const n = 10
	var wg sync.WaitGroup
	errs := make([]string, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := trySendOne(path, protocol.Request{Command: protocol.CmdPing})
			if err != nil {
				errs[idx] = err.Error()
			} else if !resp.OK {
				errs[idx] = resp.Error
			}
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != "" {
			t.Errorf("client %d: %s", i, e)
		}
	}
}

func TestBatchingClient(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	for i := range 3 {
		if err := enc.Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
			t.Fatalf("request %d encode: %v", i, err)
		}
		var resp protocol.Response
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("response %d decode: %v", i, err)
		}
		if !resp.OK {
			t.Fatalf("response %d: want ok=true, got ok=false error=%q", i, resp.Error)
		}
	}
}

func TestValidationErrorKeepsConnectionOpen(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	dec := json.NewDecoder(conn)

	if _, err := conn.Write([]byte(`{"command":"not_a_real_command"}` + "\n")); err != nil {
		t.Fatalf("write invalid: %v", err)
	}
	var resp1 protocol.Response
	if err := dec.Decode(&resp1); err != nil {
		t.Fatalf("decode resp1: %v", err)
	}
	if resp1.OK {
		t.Fatal("invalid command: want ok=false")
	}

	if err := json.NewEncoder(conn).Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	var resp2 protocol.Response
	if err := dec.Decode(&resp2); err != nil {
		t.Fatalf("decode resp2: %v", err)
	}
	if !resp2.OK {
		t.Fatalf("ping after validation error: want ok=true, got ok=false error=%q", resp2.Error)
	}
}

func TestDecodeErrorClosesConnection(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	if _, err := conn.Write([]byte("not valid json at all!!!\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.OK {
		t.Fatal("decode error: want ok=false")
	}

	var discard protocol.Response
	decErr := json.NewDecoder(conn).Decode(&discard)
	if decErr == nil {
		t.Fatal("connection should be closed after decode error, but got another response")
	}
}

func TestPanicHandlerRespondsAndContinues(t *testing.T) {
	path := sockPath(t)

	var callN atomic.Int32
	handler := func(req protocol.Request) protocol.Response {
		if callN.Add(1) == 1 {
			panic("deliberate test panic")
		}
		return protocol.Response{OK: true}
	}
	startServer(t, path, handler)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		t.Fatalf("encode first: %v", err)
	}
	var resp1 protocol.Response
	if err := dec.Decode(&resp1); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if resp1.OK {
		t.Fatal("panicking handler: want ok=false")
	}

	if err := enc.Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		t.Fatalf("encode second: %v", err)
	}
	var resp2 protocol.Response
	if err := dec.Decode(&resp2); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if !resp2.OK {
		t.Fatalf("second request after panic: want ok=true, got ok=false error=%q", resp2.Error)
	}
}

func TestContextCancellationStopsServe(t *testing.T) {
	path := sockPath(t)
	ln := mustListen(t, path, transport.Options{
		Deadline: 2 * time.Second,
		Grace:    2 * time.Second,
	})
	t.Cleanup(func() { ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, okHandler)
	}()
	waitReady(t, path)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve after cancel: want nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop within 5s after cancel")
	}
}

func TestGracefulShutdownDrainsInFlight(t *testing.T) {
	path := sockPath(t)

	slow := make(chan struct{})
	handler := func(req protocol.Request) protocol.Response {
		<-slow
		return protocol.Response{OK: true}
	}

	ln := mustListen(t, path, transport.Options{
		Deadline: 5 * time.Second,
		Grace:    3 * time.Second,
	})
	t.Cleanup(func() { ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ln.Serve(ctx, handler)
	}()
	waitReady(t, path)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	if err := json.NewEncoder(conn).Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		t.Fatalf("encode: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	cancel()

	close(slow)

	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode in-flight response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("in-flight response: want ok=true, got %v", resp)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: want nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop within 5s")
	}
}

func TestAlreadyRunning(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	_, err := transport.NewUnixListener(path, transport.Options{})
	if !errors.Is(err, transport.ErrAlreadyRunning) {
		t.Fatalf("want ErrAlreadyRunning, got %v", err)
	}

	resp := ping(t, path)
	if !resp.OK {
		t.Fatalf("first server still alive: want ok=true, got ok=false error=%q", resp.Error)
	}
}

func TestStaleSocketReclaimed(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")

	createStaleSocket(t, path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale socket file should exist after syscall.Bind: %v", err)
	}

	srv := startServer(t, path, okHandler)
	_ = srv

	resp := ping(t, path)
	if !resp.OK {
		t.Fatalf("after stale reclaim: want ok=true, got ok=false error=%q", resp.Error)
	}
}

func TestStaleSocketAndPidfile(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	createStaleSocket(t, path)

	if err := os.WriteFile(pidPath, []byte("99999\n"), 0o600); err != nil {
		t.Fatalf("write stale pidfile: %v", err)
	}

	srv := startServer(t, path, okHandler)
	_ = srv

	resp := ping(t, path)
	if !resp.OK {
		t.Fatalf("after stale reclaim with pidfile: want ok=true, got ok=false error=%q", resp.Error)
	}
}

func TestCloseSafeTwice(t *testing.T) {
	path := sockPath(t)
	ln := mustListen(t, path, transport.Options{})

	ln.Close()
	ln.Close()
}

func TestCloseSafeAfterServe(t *testing.T) {
	path := sockPath(t)
	ln := mustListen(t, path, transport.Options{
		Deadline: time.Second,
		Grace:    time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, okHandler)
	}()
	waitReady(t, path)

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}

	ln.Close()
	ln.Close()
}

func TestPidfileCreated(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	startServer(t, path, okHandler)

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		t.Fatal("pidfile is empty")
	}
	if pid == "0" {
		t.Fatal("pidfile contains zero pid")
	}
}

func TestPidfileRemovedOnClose(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	ln := mustListen(t, path, transport.Options{})
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("pidfile should exist after listen: %v", err)
	}

	ln.Close()

	if _, err := os.Stat(pidPath); err == nil {
		t.Fatal("pidfile should be removed after Close")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("socket should be removed after Close")
	}
}

func TestDoesNotRemoveLivePeerFiles(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	ln2, err := transport.NewUnixListener(path, transport.Options{})
	if !errors.Is(err, transport.ErrAlreadyRunning) {
		if ln2 != nil {
			ln2.Close()
		}
		t.Fatalf("want ErrAlreadyRunning, got %v", err)
	}

	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("first server socket removed by second NewUnixListener: %v", statErr)
	}

	resp := ping(t, path)
	if !resp.OK {
		t.Fatalf("first server should still answer after rejected second: got ok=false error=%q", resp.Error)
	}
}

func TestMessageTooLargeClosed(t *testing.T) {
	path := sockPath(t)
	startServer(t, path, okHandler)

	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	oversized := strings.Repeat("x", protocol.MaxMessageLen)
	if _, err := conn.Write([]byte(oversized)); err != nil {
		t.Fatalf("write oversized: %v", err)
	}

	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.OK {
		t.Fatal("oversized message: want ok=false")
	}

	var discard protocol.Response
	if decErr := json.NewDecoder(conn).Decode(&discard); decErr == nil {
		t.Fatal("connection should be closed after oversized message")
	}
}

func TestAcceptingButSilentSocketErrors(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")

	silent, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { silent.Close() })

	_, err = transport.NewUnixListener(path, transport.Options{})
	if err == nil {
		t.Fatal("want error for socket that accepts but does not respond")
	}
	if errors.Is(err, transport.ErrAlreadyRunning) {
		t.Fatal("want non-ErrAlreadyRunning for ambiguous probe, got ErrAlreadyRunning")
	}

	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("socket file should not be removed after ambiguous probe: %v", statErr)
	}
}

func TestRegularFileAtSocketPathRejected(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")

	if err := os.WriteFile(path, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("create file: %v", err)
	}

	_, err := transport.NewUnixListener(path, transport.Options{})
	if !errors.Is(err, transport.ErrNotSocket) {
		t.Fatalf("want ErrNotSocket, got %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file should survive: %v", err)
	}
	if string(data) != "not a socket" {
		t.Fatal("file content was modified")
	}
}

func TestCloseDoesNotRemoveReplacementSocket(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")

	ln := mustListen(t, path, transport.Options{})

	os.Remove(path)
	createStaleSocket(t, path)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("replacement socket should exist: %v", err)
	}

	ln.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Close removed replacement socket (different inode): %v", err)
	}
}

func TestClosePidfileMismatch(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	ln := mustListen(t, path, transport.Options{})

	if err := os.WriteFile(pidPath, []byte("99999\n"), 0o600); err != nil {
		t.Fatalf("overwrite pidfile: %v", err)
	}

	ln.Close()

	if _, err := os.Stat(path); err == nil {
		t.Fatal("socket should be removed by Close (same inode)")
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("pidfile with different pid must survive Close: %v", err)
	}
}

func TestClosePidfileAlreadyGone(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	ln := mustListen(t, path, transport.Options{})
	os.Remove(pidPath)

	ln.Close()

	if _, err := os.Stat(path); err == nil {
		t.Fatal("socket should be removed even when pidfile was already gone")
	}
}

func TestCloseSocketAlreadyGone(t *testing.T) {
	dir := tmpDir(t)
	path := filepath.Join(dir, "t.sock")
	pidPath := filepath.Join(dir, "humd.pid")

	ln := mustListen(t, path, transport.Options{})
	os.Remove(path)

	ln.Close()

	if _, err := os.Stat(pidPath); err == nil {
		t.Fatal("pidfile should be removed even when socket was already externally deleted")
	}
}

func TestServeRealAcceptError(t *testing.T) {
	path := sockPath(t)
	ln := mustListen(t, path, transport.Options{
		Deadline: time.Second,
		Grace:    time.Second,
	})
	t.Cleanup(func() { ln.Close() })

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, okHandler)
	}()
	waitReady(t, path)

	ln.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Serve must return a non-nil error when listener is closed without context cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

func TestOptionsAllSet(t *testing.T) {
	path := sockPath(t)
	opts := transport.Options{
		Logger:   slog.Default(),
		Deadline: 3 * time.Second,
		Grace:    3 * time.Second,
	}
	ln := mustListen(t, path, opts)
	t.Cleanup(func() { ln.Close() })

	if ln.Addr() != path {
		t.Fatalf("Addr: want %q, got %q", path, ln.Addr())
	}
}

func TestSocketPathTooLongIsRejectedClearly(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("x", 120))

	_, err := transport.NewUnixListener(long, transport.Options{})
	if !errors.Is(err, transport.ErrSocketPathTooLong) {
		t.Fatalf("NewUnixListener with a %d byte path = %v, want ErrSocketPathTooLong", len(long), err)
	}
	if !strings.Contains(err.Error(), "limit is") {
		t.Errorf("error %q does not state the limit", err)
	}
	if _, statErr := os.Stat(long); statErr == nil {
		t.Error("the rejected path was created")
	}
}
