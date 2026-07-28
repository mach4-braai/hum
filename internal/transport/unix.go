package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

var ErrAlreadyRunning = errors.New("daemon already running")
var ErrRelativeSocket = errors.New("socket path must be absolute")
var ErrNotSocket = errors.New("path exists and is not a Unix socket")
var ErrSocketPathTooLong = errors.New("socket path is too long for a Unix socket address")

var (
	osMkdirAll  = os.MkdirAll
	osChmod     = os.Chmod
	osStat      = os.Stat
	osWriteFile = os.WriteFile
	netListen   = net.Listen
	probeDialer = func(path string) (net.Conn, error) {
		return net.DialTimeout("unix", path, 2*time.Second)
	}
	connSetDeadline = func(conn net.Conn, t time.Time) error {
		return conn.SetDeadline(t)
	}
)

type unixListener struct {
	path      string
	pidPath   string
	ln        net.Listener
	opts      Options
	sockInfo  os.FileInfo
	closeOnce sync.Once
}

func NewUnixListener(path string, opts Options) (Listener, error) {
	opts.applyDefaults()

	if !filepath.IsAbs(path) {
		return nil, ErrRelativeSocket
	}

	if len(path) >= maxSocketPathLen {
		return nil, fmt.Errorf("%w: %d bytes, limit is %d: %s", ErrSocketPathTooLong, len(path), maxSocketPathLen-1, path)
	}

	if info, err := osStat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%w: %s", ErrNotSocket, path)
		}
		live, err := probeLive(path)
		if err != nil {
			return nil, fmt.Errorf("probe socket: %w", err)
		}
		if live {
			return nil, ErrAlreadyRunning
		}
		pidPath := filepath.Join(filepath.Dir(path), "humd.pid")
		os.Remove(path)
		os.Remove(pidPath)
	}

	if err := osMkdirAll(filepath.Dir(path), paths.RuntimeDirPerm); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	ln, err := netListen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", path, err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)

	if err := osChmod(path, 0o600); err != nil {
		ln.Close()
		os.Remove(path)
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	sockInfo, err := osStat(path)
	if err != nil {
		ln.Close()
		os.Remove(path)
		return nil, fmt.Errorf("stat socket after bind: %w", err)
	}

	pidPath := filepath.Join(filepath.Dir(path), "humd.pid")
	pidData := []byte(strconv.Itoa(os.Getpid()) + "\n")
	if err := osWriteFile(pidPath, pidData, 0o600); err != nil {
		ln.Close()
		os.Remove(path)
		return nil, fmt.Errorf("write pidfile: %w", err)
	}

	return &unixListener{
		path:     path,
		pidPath:  pidPath,
		ln:       ln,
		opts:     opts,
		sockInfo: sockInfo,
	}, nil
}

func probeLive(path string) (bool, error) {
	conn, err := probeDialer(path)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) {
			return false, nil
		}
		return false, fmt.Errorf("dial %s: %w", path, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return false, fmt.Errorf("set deadline on %s: %w", path, err)
	}
	if err := json.NewEncoder(conn).Encode(protocol.Request{Command: protocol.CmdPing}); err != nil {
		return false, fmt.Errorf("ping write to %s: %w", path, err)
	}
	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return false, fmt.Errorf("ping read from %s: %w", path, err)
	}
	return true, nil
}

func (l *unixListener) Addr() string {
	return l.path
}

func (l *unixListener) Close() error {
	l.closeOnce.Do(func() {
		l.ln.Close()
		if cur, err := os.Stat(l.path); err == nil && os.SameFile(l.sockInfo, cur) {
			os.Remove(l.path)
		}
		if data, err := os.ReadFile(l.pidPath); err == nil {
			if strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
				os.Remove(l.pidPath)
			}
		}
	})
	return nil
}

func (l *unixListener) Serve(ctx context.Context, h Handler) error {
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-ctx.Done():
			l.ln.Close()
		case <-stop:
		}
	}()

	var wg sync.WaitGroup

	for {
		conn, err := l.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				done := make(chan struct{})
				go func() {
					wg.Wait()
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(l.opts.Grace):
				}
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			l.handleConn(conn, h)
		}()
	}
}

func (l *unixListener) handleConn(conn net.Conn, h Handler) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, protocol.MaxMessageLen), protocol.MaxMessageLen)
	enc := json.NewEncoder(conn)

	for {
		if err := connSetDeadline(conn, time.Now().Add(l.opts.Deadline)); err != nil {
			return
		}

		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				return
			}
			if errors.Is(err, bufio.ErrTooLong) {
				connSetDeadline(conn, time.Now().Add(l.opts.Deadline))
				enc.Encode(protocol.Response{OK: false, Error: protocol.ErrMessageTooLarge.Error()})
			}
			return
		}

		var req protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			connSetDeadline(conn, time.Now().Add(l.opts.Deadline))
			enc.Encode(protocol.Response{OK: false, Error: "invalid request"})
			return
		}

		if err := req.Validate(); err != nil {
			connSetDeadline(conn, time.Now().Add(l.opts.Deadline))
			enc.Encode(protocol.Response{OK: false, Error: err.Error()})
			continue
		}

		resp := safeHandle(h, req, l.opts)
		connSetDeadline(conn, time.Now().Add(l.opts.Deadline))
		enc.Encode(resp)
	}
}

func safeHandle(h Handler, req protocol.Request, opts Options) (resp protocol.Response) {
	defer func() {
		if r := recover(); r != nil {
			opts.Logger.Error("handler panic", "recover", fmt.Sprintf("%v", r))
			resp = protocol.Response{OK: false, Error: "internal error"}
		}
	}()
	return h(req)
}
