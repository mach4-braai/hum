package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

func seampDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "humts")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func seamSockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(seampDir(t), "t.sock")
}

func seamStaleSocket(t *testing.T, path string) {
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

func seamWaitReady(t *testing.T, path string) {
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

type mockConn struct {
	deadlineErr error
	writeErr    error
	readErr     error
}

func (m *mockConn) SetDeadline(_ time.Time) error      { return m.deadlineErr }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return &net.UnixAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.UnixAddr{} }
func (m *mockConn) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}
func (m *mockConn) Read(p []byte) (int, error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	return 0, io.EOF
}

func TestNewUnixListenerMkdirFails(t *testing.T) {
	orig := osMkdirAll
	t.Cleanup(func() { osMkdirAll = orig })
	osMkdirAll = func(_ string, _ os.FileMode) error {
		return errors.New("injected mkdir failure")
	}

	path := seamSockPath(t)
	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error from osMkdirAll failure")
	}
}

func TestNewUnixListenerListenFails(t *testing.T) {
	orig := netListen
	t.Cleanup(func() { netListen = orig })
	netListen = func(_, _ string) (net.Listener, error) {
		return nil, errors.New("injected listen failure")
	}

	path := seamSockPath(t)
	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error from netListen failure")
	}
}

func TestNewUnixListenerChmodFails(t *testing.T) {
	orig := osChmod
	t.Cleanup(func() { osChmod = orig })
	osChmod = func(_ string, _ os.FileMode) error {
		return errors.New("injected chmod failure")
	}

	path := seamSockPath(t)
	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error from osChmod failure")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("socket file should be cleaned up after osChmod failure")
	}
}

func TestNewUnixListenerStatAfterBindFails(t *testing.T) {
	callN := 0
	orig := osStat
	t.Cleanup(func() { osStat = orig })
	osStat = func(name string) (os.FileInfo, error) {
		callN++
		if callN == 1 {
			return nil, errors.New("injected stat failure (pre-bind check)")
		}
		if callN == 2 {
			return nil, errors.New("injected stat failure (post-bind)")
		}
		return orig(name)
	}

	path := seamSockPath(t)
	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error from osStat failure after bind")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("socket file should be cleaned up after osStat failure")
	}
}

func TestNewUnixListenerPidfileWriteFails(t *testing.T) {
	orig := osWriteFile
	t.Cleanup(func() { osWriteFile = orig })
	osWriteFile = func(_ string, _ []byte, _ os.FileMode) error {
		return errors.New("injected writeFile failure")
	}

	path := seamSockPath(t)
	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error from osWriteFile failure")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("socket file should be cleaned up after osWriteFile failure")
	}
}

func TestProbeLiveNonECONNREFUSEDError(t *testing.T) {
	orig := probeDialer
	t.Cleanup(func() { probeDialer = orig })
	probeDialer = func(_ string) (net.Conn, error) {
		return nil, syscall.EPERM
	}

	dir := seampDir(t)
	path := filepath.Join(dir, "t.sock")
	seamStaleSocket(t, path)

	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error for non-ECONNREFUSED dial error")
	}
	if errors.Is(err, ErrAlreadyRunning) {
		t.Fatal("want non-ErrAlreadyRunning error")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("socket file must survive when probe returns an ambiguous error: %v", statErr)
	}
}

func TestProbeLiveSetDeadlineFails(t *testing.T) {
	orig := probeDialer
	t.Cleanup(func() { probeDialer = orig })
	probeDialer = func(_ string) (net.Conn, error) {
		return &mockConn{deadlineErr: errors.New("fake deadline failure")}, nil
	}

	dir := seampDir(t)
	path := filepath.Join(dir, "t.sock")
	seamStaleSocket(t, path)

	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error when probe conn SetDeadline fails")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("socket file must survive: %v", statErr)
	}
}

func TestProbeLiveEncodeFails(t *testing.T) {
	orig := probeDialer
	t.Cleanup(func() { probeDialer = orig })
	probeDialer = func(_ string) (net.Conn, error) {
		return &mockConn{writeErr: errors.New("fake write failure")}, nil
	}

	dir := seampDir(t)
	path := filepath.Join(dir, "t.sock")
	seamStaleSocket(t, path)

	_, err := NewUnixListener(path, Options{})
	if err == nil {
		t.Fatal("want error when probe conn Write fails")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("socket file must survive: %v", statErr)
	}
}

func TestHandleConnSetDeadlineFails(t *testing.T) {
	orig := connSetDeadline
	t.Cleanup(func() { connSetDeadline = orig })
	connSetDeadline = func(_ net.Conn, _ time.Time) error {
		return errors.New("injected deadline failure")
	}

	path := seamSockPath(t)
	ln, err := NewUnixListener(path, Options{Deadline: time.Second, Grace: time.Second})
	if err != nil {
		t.Fatalf("NewUnixListener: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, func(_ protocol.Request) protocol.Response {
			return protocol.Response{OK: true}
		})
	}()
	seamWaitReady(t, path)

	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	var resp protocol.Response
	if json.NewDecoder(conn).Decode(&resp) == nil {
		t.Fatal("expected connection close without response when deadline seam fails")
	}

	cancel()
	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("Serve: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

func TestHandleConnScannerTimeoutNotTooLarge(t *testing.T) {
	orig := connSetDeadline
	t.Cleanup(func() { connSetDeadline = orig })
	connSetDeadline = func(conn net.Conn, _ time.Time) error {
		return conn.SetDeadline(time.Now().Add(-time.Second))
	}

	path := seamSockPath(t)
	ln, err := NewUnixListener(path, Options{Deadline: time.Second, Grace: time.Second})
	if err != nil {
		t.Fatalf("NewUnixListener: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ln.Serve(ctx, func(_ protocol.Request) protocol.Response {
			return protocol.Response{OK: true}
		})
	}()
	seamWaitReady(t, path)

	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	var resp protocol.Response
	if json.NewDecoder(conn).Decode(&resp) == nil {
		t.Fatal("expected connection close: past deadline should cause scanner timeout, not a response")
	}

	cancel()
	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("Serve: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}
