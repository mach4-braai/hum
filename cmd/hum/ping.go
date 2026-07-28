package main

import "github.com/mach4-braai/hum/internal/protocol"

func init() {
	register("ping", runPing)
}

func runPing(e *env, words []string) int {
	rest, ok := operands(e, "ping", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "ping", rest[0])
	}
	return send(e, protocol.Request{Command: protocol.CmdPing})
}
