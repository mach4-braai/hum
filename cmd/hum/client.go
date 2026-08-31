package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

func setConnDeadline(c net.Conn, t time.Time) error {
	return c.SetDeadline(t)
}

var connSetDeadline = setConnDeadline

var errNoDaemon = errors.New("no daemon listening")
var errSocketMissing = fmt.Errorf("%w", errNoDaemon)
var errSocketStale = fmt.Errorf("%w", errNoDaemon)
var errSocketDenied = errors.New("socket access denied")

type answer struct {
	raw      json.RawMessage
	response protocol.Response
}

func query(request protocol.Request, timeout time.Duration) (answer, error) {
	socket := paths.SocketPath()
	conn, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return answer{}, classifyDialError(err, socket)
	}
	defer conn.Close()

	if err := connSetDeadline(conn, time.Now().Add(timeout)); err != nil {
		return answer{}, fmt.Errorf("cannot set a deadline on %s: %w", socket, err)
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return answer{}, fmt.Errorf("cannot send the request to %s: %w", socket, err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return answer{}, fmt.Errorf("no usable response from %s: %w", socket, err)
	}

	var response protocol.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return answer{}, fmt.Errorf("malformed response from %s: %w", socket, err)
	}
	return answer{raw: raw, response: response}, nil
}

func unreachable(e *env, err error) int {
	if errors.Is(err, errSocketDenied) {
		fmt.Fprintf(e.stderr, "hum: %v\nrun `hum doctor` to diagnose ownership and permissions\n", err)
	} else if errors.Is(err, errNoDaemon) {
		fmt.Fprintf(e.stderr, "hum: %v\nstart it with `humd`, or `brew services start hum`\n", err)
	} else {
		fmt.Fprintf(e.stderr, "hum: %v\n", err)
	}
	return exitUnreachable
}

func send(e *env, request protocol.Request) int {
	got, err := query(request, e.opts.timeout)
	if err != nil {
		return unreachable(e, err)
	}
	if e.opts.asJSON {
		printJSON(e, got.raw)
	}
	if !got.response.OK {
		return e.fail("hum: %s", responseError(got.response))
	}
	return exitOK
}

func fetchPayload(e *env, request protocol.Request, into any) (json.RawMessage, int) {
	got, err := query(request, e.opts.timeout)
	if err != nil {
		return nil, unreachable(e, err)
	}
	if !got.response.OK {
		return nil, e.fail("hum: %s", responseError(got.response))
	}
	if err := json.Unmarshal(got.response.Data, into); err != nil {
		fmt.Fprintf(e.stderr, "hum: malformed %s payload: %v\n", request.Command, err)
		return nil, exitUnreachable
	}
	return got.response.Data, exitOK
}

func fetchStatus(e *env) (protocol.StatusPayload, json.RawMessage, int) {
	var status protocol.StatusPayload
	raw, code := fetchPayload(e, protocol.Request{Command: protocol.CmdStatus}, &status)
	return status, raw, code
}

func printJSON(e *env, raw json.RawMessage) {
	fmt.Fprintln(e.stdout, strings.TrimSpace(string(raw)))
}

func responseError(response protocol.Response) string {
	if response.Error == "" {
		return "the daemon reported a failure without an error message"
	}
	return response.Error
}
