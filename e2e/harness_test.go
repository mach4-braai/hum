//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

var binaries = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "hum-e2e")
	if err != nil {
		return "", err
	}
	for _, target := range []string{"hum", "humd"} {
		build := exec.Command("go", "build", "-o", filepath.Join(dir, target+exeSuffix), "../cmd/"+target)
		if out, err := build.CombinedOutput(); err != nil {
			return "", errors.New(string(out))
		}
	}
	return dir, nil
})

type daemon struct {
	cmd    *exec.Cmd
	bin    string
	home   string
	socket string
	lines  chan string
}

func start(t *testing.T, args ...string) *daemon {
	t.Helper()

	dir, err := binaries()
	if err != nil {
		t.Fatalf("build binaries: %v", err)
	}

	home, err := os.MkdirTemp("", "he")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	socket := filepath.Join(home, "humd.sock")

	full := append([]string{"--no-audio", "--log-level", "debug", "--socket", socket}, args...)
	cmd := exec.Command(filepath.Join(dir, "humd"+exeSuffix), full...)
	cmd.Env = append(os.Environ(), "HUM_HOME="+home, "HUM_SOCKET="+socket)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start humd: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})

	lines := make(chan string, 4096)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	d := &daemon{cmd: cmd, bin: dir, home: home, socket: socket, lines: lines}
	d.waitForSocket(t)
	return d
}

func (d *daemon) waitForSocket(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(d.socket); err == nil && info.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("humd did not bind %s within 10s", d.socket)
}

func (d *daemon) hum(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(d.bin, "hum"+exeSuffix), args...)
	cmd.Env = append(os.Environ(), "HUM_HOME="+d.home, "HUM_SOCKET="+d.socket)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("hum %s: %v", strings.Join(args, " "), err)
	}
	return string(out), exitErr.ExitCode()
}

func (d *daemon) mustHum(t *testing.T, args ...string) string {
	t.Helper()
	out, code := d.hum(t, args...)
	if code != 0 {
		t.Fatalf("hum %s exited %d: %s", strings.Join(args, " "), code, out)
	}
	return out
}

func (d *daemon) status(t *testing.T) protocol.StatusPayload {
	t.Helper()
	out := d.mustHum(t, "status", "--json")
	var st protocol.StatusPayload
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	return st
}

func (d *daemon) waitForLog(t *testing.T, substring string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case line, ok := <-d.lines:
			if !ok {
				t.Fatalf("humd stderr closed before logging %q", substring)
			}
			if strings.Contains(line, substring) {
				return
			}
		case <-deadline:
			t.Fatalf("humd did not log %q within %v", substring, within)
		}
	}
}

func (d *daemon) waitExit(t *testing.T, within time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- d.cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("wait: %v", err)
		}
		return exitErr.ExitCode()
	case <-time.After(within):
		d.cmd.Process.Kill()
		t.Fatalf("humd did not exit within %v", within)
	}
	return -1
}

func pitches(st protocol.StatusPayload) map[string]string {
	out := make(map[string]string, len(st.Sessions))
	for _, s := range st.Sessions {
		out[s.ID] = s.Pitch
	}
	return out
}

func writeProjectConfig(t *testing.T, body string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hp")
	if err != nil {
		t.Fatalf("temp project: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.MkdirAll(filepath.Join(dir, ".hum"), 0o700); err != nil {
		t.Fatalf("mkdir .hum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hum", "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return resolved
}

func (d *daemon) sendRaw(t *testing.T, line string) protocol.Response {
	t.Helper()
	conn, err := net.Dial("unix", d.socket)
	if err != nil {
		t.Fatalf("dial %s: %v", d.socket, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("no response to %q: %v", line, scanner.Err())
	}
	var resp protocol.Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", scanner.Text(), err)
	}
	return resp
}

func symlink(t *testing.T, target string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hs")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return link
}
