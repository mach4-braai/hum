package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

var buildHumd = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "humd-bin")
	if err != nil {
		return "", err
	}
	binary := filepath.Join(dir, "humd"+exeSuffix)
	build := exec.Command("go", "build", "-o", binary, "../humd")
	if out, err := build.CombinedOutput(); err != nil {
		return "", errors.New(string(out))
	}
	return binary, nil
})

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type daemonProcess struct {
	t      *testing.T
	socket string
	binary string
	args   []string
	cmd    *exec.Cmd
	out    *lockedBuffer
	waited bool
}

func startHumd(t *testing.T, args ...string) *daemonProcess {
	t.Helper()

	dir, err := os.MkdirTemp("", "hd")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	binary, err := buildHumd()
	if err != nil {
		t.Fatalf("build humd: %v", err)
	}

	socket := filepath.Join(dir, "humd.sock")
	t.Setenv("HUM_SOCKET", socket)

	d := &daemonProcess{
		t:      t,
		socket: socket,
		binary: binary,
		args:   append([]string{"--no-audio", "--socket", socket, "--log-level", "debug"}, args...),
	}
	d.start()
	t.Cleanup(d.stop)
	return d
}

func (d *daemonProcess) start() {
	d.t.Helper()

	d.out = &lockedBuffer{}
	d.cmd = exec.Command(d.binary, d.args...)
	d.cmd.Env = os.Environ()
	d.cmd.Stdout = d.out
	d.cmd.Stderr = d.out
	if err := d.cmd.Start(); err != nil {
		d.t.Fatalf("start humd: %v", err)
	}
	d.waited = false
	d.waitForSocket()
}

func (d *daemonProcess) waitForSocket() {
	d.t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", d.socket)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	d.t.Fatalf("humd never accepted a connection on %s\n%s", d.socket, d.out.String())
}

func (d *daemonProcess) stop() {
	if d.waited {
		return
	}
	d.waited = true
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		d.cmd.Process.Kill()
	}
	d.cmd.Wait()
}

func (d *daemonProcess) restart() {
	d.t.Helper()
	d.stop()
	d.start()
}

func (d *daemonProcess) logs() string {
	return d.out.String()
}

func TestPingReachesARealDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	startHumd(t)
	code, out := runBinary(t, os.Getenv("HUM_SOCKET"), "ping")

	if code != exitOK {
		t.Fatalf("hum ping exited %d, want %d\n%s", code, exitOK, out)
	}
}
