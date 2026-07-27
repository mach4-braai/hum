package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

// send performs one control round trip: dial, write one request, read one
// response. The connection is not reused, so a wedged daemon cannot leave the
// CLI holding a half-consumed stream.
func send(request protocol.Request, timeout time.Duration, asJSON bool, stdout, stderr io.Writer) int {
	socket := paths.SocketPath()
	conn, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		// Never the raw dial error: "connect: no such file or directory" tells
		// a user nothing about what to do next.
		fmt.Fprintf(stderr, "hum: no daemon listening at %s\nstart it with `humd`, or `brew services start hum`\n", socket)
		return exitUnreachable
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		fmt.Fprintf(stderr, "hum: cannot set a deadline on %s: %v\n", socket, err)
		return exitUnreachable
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		fmt.Fprintf(stderr, "hum: cannot send the request to %s: %v\n", socket, err)
		return exitUnreachable
	}

	// Decoded raw first, so --json can print exactly what the daemon sent
	// rather than a re-encoding that may differ in field order or omissions.
	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		fmt.Fprintf(stderr, "hum: no usable response from %s: %v\n", socket, err)
		return exitUnreachable
	}
	if asJSON {
		fmt.Fprintln(stdout, string(raw))
	}

	var response protocol.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		fmt.Fprintf(stderr, "hum: malformed response from %s: %v\n", socket, err)
		return exitUnreachable
	}
	if !response.OK {
		fmt.Fprintf(stderr, "hum: %s\n", responseError(response))
		return exitDaemonError
	}
	if !asJSON && len(response.Data) > 0 {
		fmt.Fprintln(stdout, string(response.Data))
	}
	return exitOK
}

// responseError keeps a failing response from printing an empty line when the
// daemon reports failure without saying why.
func responseError(response protocol.Response) string {
	if response.Error == "" {
		return "the daemon reported a failure without an error message"
	}
	return response.Error
}
